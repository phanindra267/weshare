import { test, expect, type Page } from "@playwright/test";

/**
 * Live end-to-end group messaging: three users become contacts, the owner
 * creates a group, invites them, and shares an image that the members receive.
 *
 * Also proves the authorization boundary: a non-member cannot read the group's
 * messages (the API answers 404 rather than disclosing it exists).
 *
 * Requires the API on http://localhost:8080 with a migrated database (see
 * docs/TESTING.md). The Next dev server is started by the Playwright config.
 */

const API = "http://localhost:8080";
const PASSWORD = "sup3r-secret";

/** A 1x1 PNG, valid enough for the server's content sniffing. */
const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

let userSeq = 0;

function uniqueUser(prefix: string) {
  userSeq += 1;
  const stamp = `${Date.now()}${userSeq}`;
  return {
    email: `${prefix}${stamp}@example.com`,
    username: `${prefix}${stamp}`.slice(0, 32),
    displayName: `${prefix}-${userSeq}`,
  };
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

/**
 * The centered system notice in the thread. Scoped to `p` because the same
 * text also appears in the sidebar's last-message preview (a `<span>`).
 */
function systemNotice(page: Page, text: RegExp) {
  return page.locator("p").filter({ hasText: text });
}

test.describe("group messaging end to end", () => {
  // The dev server is pre-warmed by global-setup, so this budget covers the
  // four registrations plus the group and media round-trip with headroom.
  test.describe.configure({ timeout: 120_000 });

  test("owner creates a group, shares an image, and members receive it", async ({
    browser,
  }) => {
    const aliceCtx = await browser.newContext();
    const bobCtx = await browser.newContext();
    const carolCtx = await browser.newContext();
    const daveCtx = await browser.newContext();
    const alice = await aliceCtx.newPage();
    const bob = await bobCtx.newPage();
    const carol = await carolCtx.newPage();
    const dave = await daveCtx.newPage();

    try {
      const aliceUser = uniqueUser("alice");
      const bobUser = uniqueUser("bob");
      const carolUser = uniqueUser("carol");
      const daveUser = uniqueUser("dave");

      await register(alice, aliceUser);
      await register(bob, bobUser);
      await register(carol, carolUser);
      await register(dave, daveUser);

      const bobId = await myId(bob);
      const carolId = await myId(carol);
      await sendFriendRequest(alice, bobId);
      await acceptFriendRequest(bob, aliceUser.displayName);
      await sendFriendRequest(alice, carolId);
      await acceptFriendRequest(carol, aliceUser.displayName);

      // Alice creates a group with Bob and Carol.
      await alice.goto("/messages");
      await alice.getByRole("button", { name: "New group" }).click();
      await alice.getByLabel("Group name").fill("Project X");
      await alice
        .getByRole("checkbox", { name: new RegExp(bobUser.displayName) })
        .check();
      await alice
        .getByRole("checkbox", { name: new RegExp(carolUser.displayName) })
        .check();
      await alice.getByRole("button", { name: "Create group" }).click();

      // The new group is selected and announces itself with a system message.
      await expect(systemNotice(alice, /created the group/)).toBeVisible({
        timeout: 15_000,
      });

      // Bob sees the group appear live, without reloading the page.
      await bob.goto("/messages");
      await expect(bob.getByRole("button", { name: "Project X" })).toBeVisible({
        timeout: 15_000,
      });
      await bob.getByRole("button", { name: "Project X" }).click();
      await expect(systemNotice(bob, /created the group/)).toBeVisible();

      // Alice shares an image; Bob receives it in real time.
      const png = Buffer.from(PNG_BASE64, "base64");
      await alice
        .locator('input[type="file"]')
        .setInputFiles({ name: "pic.png", mimeType: "image/png", buffer: png });
      await expect(alice.getByAltText("Shared image").first()).toBeVisible({
        timeout: 20_000,
      });
      await expect(bob.getByAltText("Shared image").first()).toBeVisible({
        timeout: 20_000,
      });

      // Bob is a plain member: the info dialog shows members but no admin tools.
      await bob.getByRole("button", { name: "Group info" }).click();
      await expect(
        bob.getByRole("heading", { name: "Group info" }),
      ).toBeVisible();
      await expect(bob.getByRole("heading", { name: "Members" })).toBeVisible();
      await expect(
        bob.getByRole("button", { name: "Add members" }),
      ).toHaveCount(0);
      await bob.keyboard.press("Escape");
      await expect(
        bob.getByRole("heading", { name: "Group info" }),
      ).toHaveCount(0);

      // A non-member cannot read the group's history: the API answers 404.
      const conversations = await alice.evaluate(async (api: string) => {
        const res = await fetch(`${api}/api/v1/conversations`, {
          credentials: "include",
        });
        const body = (await res.json()) as {
          data: { items: { id: string; title: string | null }[] };
        };
        return body.data.items;
      }, API);
      const group = conversations.find((c) => c.title === "Project X");
      expect(group).toBeTruthy();

      const status = await dave.evaluate(
        async ({ api, id }: { api: string; id: string }) => {
          const res = await fetch(
            `${api}/api/v1/conversations/${id}/messages`,
            { credentials: "include" },
          );
          return res.status;
        },
        { api: API, id: group!.id },
      );
      expect(status).toBe(404);
    } finally {
      await aliceCtx.close();
      await bobCtx.close();
      await carolCtx.close();
      await daveCtx.close();
    }
  });
});
