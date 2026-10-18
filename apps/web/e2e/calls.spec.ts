import { test, expect, type Page } from "@playwright/test";

/**
 * Live end-to-end calling (Phase 9): two contacts place a real WebRTC call
 * over the signalling socket and stream media directly to each other.
 *
 * The test asserts that both sides reach `active`, a state only reachable once
 * a peer connection reports `connected`, and that the remote video element
 * is actually decoding frames. That distinguishes a real negotiation from two
 * browsers that merely agreed to disagree.
 *
 * Requires the API on http://localhost:8080 with a migrated database (see
 * docs/TESTING.md). Chromium is launched with fake-media flags by
 * playwright.config.ts, so no camera or microphone is needed.
 */

const API = "http://localhost:8080";
const PASSWORD = "sup3r-secret";

/**
 * Identifies this worker process.
 *
 * Playwright runs spec files in parallel workers, each with its own copy of
 * this module, so `Date.now()` alone does not keep generated usernames apart:
 * two workers that start in the same millisecond would both produce
 * `alice<stamp>1` and the second registration would fail as "already taken".
 * Mixing in a per-process random suffix makes collisions impossible.
 */
const RUN_ID = Math.random().toString(36).slice(2, 8);
let userSeq = 0;

function uniqueUser(prefix: string) {
  userSeq += 1;
  const stamp = `${Date.now().toString(36)}${userSeq}${RUN_ID}`;
  return {
    email: `${prefix}${stamp}@example.com`,
    username: `${prefix}${stamp}`.slice(0, 32),
    displayName: `${prefix}-${userSeq}`,
  };
}

/**
 * Closes contexts without letting a slow teardown mask the real failure.
 *
 * A hung `close()` after a failed assertion would otherwise eat the whole test
 * timeout and replace the useful error with "test timeout exceeded".
 */
async function closeQuietly(contexts: Array<{ close: () => Promise<void> }>) {
  await Promise.race([
    Promise.allSettled(contexts.map((context) => context.close())),
    new Promise((resolve) => setTimeout(resolve, 10_000)),
  ]);
}

async function register(page: Page, user: ReturnType<typeof uniqueUser>) {
  await page.goto("/register");
  await page.getByLabel("Email").fill(user.email);
  await page.getByLabel("Username").fill(user.username);
  await page.getByLabel("Display name").fill(user.displayName);
  await page.getByLabel("Password").fill(PASSWORD);
  await page
    .locator("#main")
    .getByRole("button", { name: "Create account" })
    .click();
  await page.waitForURL("**/profile");
}

async function myId(page: Page): Promise<string> {
  return page.evaluate(async (api: string) => {
    const res = await fetch(`${api}/api/v1/auth/me`, {
      credentials: "include",
    });
    const body = (await res.json()) as { data: { id: string } };
    return body.data.id;
  }, API);
}

async function sendFriendRequest(page: Page, toId: string) {
  await page.goto("/contacts");
  await page.getByLabel("User id").fill(toId);
  await page.getByRole("button", { name: "Send request" }).click();
  await expect(page.getByText("Contact request sent")).toBeVisible();
}

async function acceptFriendRequest(page: Page, fromName: string) {
  await page.goto("/contacts");
  await page.getByRole("button", { name: "Accept" }).first().click();
  await expect(page.getByText(fromName)).toBeVisible();
}

/** Reads the caller's stored call history straight from the API. */
async function callHistory(page: Page) {
  return page.evaluate(async (api: string) => {
    const res = await fetch(`${api}/api/v1/calls?limit=10`, {
      credentials: "include",
    });
    const body = (await res.json()) as {
      data?: { items: Array<Record<string, unknown>> };
    };
    return body.data?.items ?? [];
  }, API);
}

/** Waits until a video element is painting decoded frames from its stream. */
async function expectFrames(page: Page, timeout = 30_000) {
  await expect
    .poll(
      async () => {
        const video = page.getByTestId("remote-peers").locator("video").first();
        if ((await video.count()) === 0) return 0;
        return video.evaluate((element: HTMLVideoElement) => element.videoWidth);
      },
      { timeout, intervals: [500] },
    )
    .toBeGreaterThan(0);
}

test.describe("calls end to end", () => {
  test.describe.configure({ timeout: 180_000 });

  test("two contacts negotiate a live call and record it", async ({
    browser,
  }) => {
    const aliceCtx = await browser.newContext();
    const bobCtx = await browser.newContext();
    const alice = await aliceCtx.newPage();
    const bob = await bobCtx.newPage();

    try {
      const aliceUser = uniqueUser("alice");
      const bobUser = uniqueUser("bob");

      await register(alice, aliceUser);
      await register(bob, bobUser);

      const bobId = await myId(bob);
      await sendFriendRequest(alice, bobId);
      await acceptFriendRequest(bob, aliceUser.displayName);

      // Alice places a video call from Bob's contact row. The call id is
      // generated on the client, so the UI reflects it before the server does.
      await alice.goto("/contacts");
      await alice
        .getByRole("button", { name: `Video call ${bobUser.displayName}` })
        .click();
      await expect(alice.getByTestId("call-state")).toHaveText(
        "outgoing-ringing",
        { timeout: 15_000 },
      );
      await expect(
        alice.getByText(`In call with ${bobUser.displayName}`),
      ).toBeVisible();

      // Bob's socket carries the invite to the other browser context.
      await expect(bob.getByTestId("call-incoming")).toBeVisible({
        timeout: 30_000,
      });
      await expect(bob.getByTestId("call-incoming")).toContainText(
        aliceUser.displayName,
      );
      await bob.getByTestId("call-accept").click();

      // `active` is only set from `connectionState === "connected"`, so this
      // proves ICE and DTLS completed between the two browsers.
      await expect(alice.getByTestId("call-state")).toHaveText("active", {
        timeout: 60_000,
      });
      await expect(bob.getByTestId("call-state")).toHaveText("active", {
        timeout: 60_000,
      });

      // Remote video is attached on both sides and frames are decoding, so
      // media really did travel peer-to-peer.
      await expect(
        alice.getByTestId("remote-peers").locator("video"),
      ).toHaveCount(1);
      await expect(bob.getByTestId("remote-peers").locator("video")).toHaveCount(
        1,
      );
      await expectFrames(alice);
      await expectFrames(bob);

      // Alice hangs up; both sides tear down and the call is persisted.
      await alice.getByTestId("call-end").click();
      await expect(alice.getByTestId("call-state")).toHaveCount(0);
      await expect(bob.getByTestId("call-state")).toHaveCount(0);

      await expect
        .poll(async () => (await callHistory(alice)).length, {
          timeout: 15_000,
        })
        .toBeGreaterThan(0);

      const history = await callHistory(alice);
      expect(history[0]?.kind).toBe("video");
      expect(history[0]?.direction).toBe("outgoing");
      expect(["active", "ended"]).toContain(history[0]?.status);
      const participants = history[0]?.participants as Array<{
        display_name: string;
      }>;
      expect(participants.map((p) => p.display_name)).toContain(
        bobUser.displayName,
      );
    } finally {
      await closeQuietly([aliceCtx, bobCtx]);
    }
  });

  test("a callee can decline, leaving no call behind", async ({ browser }) => {
    const aliceCtx = await browser.newContext();
    const bobCtx = await browser.newContext();
    const alice = await aliceCtx.newPage();
    const bob = await bobCtx.newPage();

    try {
      const aliceUser = uniqueUser("alice");
      const bobUser = uniqueUser("bob");

      await register(alice, aliceUser);
      await register(bob, bobUser);

      const bobId = await myId(bob);
      await sendFriendRequest(alice, bobId);
      await acceptFriendRequest(bob, aliceUser.displayName);

      await alice.goto("/contacts");
      await alice
        .getByRole("button", { name: `Call ${bobUser.displayName}`, exact: true })
        .click();

      await expect(
        bob.getByTestId("call-incoming"),
      ).toBeVisible({ timeout: 30_000 });
      await bob.getByTestId("call-reject").click();

      // Both sides drop back to idle and no overlay is left behind.
      await expect(alice.getByTestId("call-state")).toHaveCount(0, {
        timeout: 30_000,
      });
      await expect(bob.getByTestId("call-state")).toHaveCount(0);

      const history = await callHistory(alice);
      expect(history).toHaveLength(1);
      expect(history[0]?.kind).toBe("audio");
      expect(["rejected", "declined", "missed"]).toContain(history[0]?.status);
    } finally {
      await closeQuietly([aliceCtx, bobCtx]);
    }
  });
});
