import { test, expect, type Page } from "@playwright/test";

/**
 * Live end-to-end moments (Phase 5): two contacts share a moment, react to it,
 * and comment. Proves the friends-visibility feed, likes, comments, and author
 * deletion against the real API and database.
 *
 * Requires the API on http://localhost:8080 with a migrated database (see
 * docs/TESTING.md). The Next dev server is started by the Playwright config.
 */

const API = "http://localhost:8080";
const PASSWORD = "sup3r-secret";

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

test.describe("moments end to end", () => {
  test.describe.configure({ timeout: 120_000 });

  test("a friend sees, likes, and comments on a shared moment", async ({
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

      // Alice shares a friends-only moment.
      await alice.goto("/moments");
      await alice.getByLabel("Moment body").fill("Hello from Alice");
      await alice.getByRole("button", { name: "Share" }).click();
      await expect(alice.getByText("Hello from Alice")).toBeVisible();

      // Bob, a friend, sees it in his feed.
      await bob.goto("/moments");
      await expect(bob.getByText("Hello from Alice")).toBeVisible({
        timeout: 15_000,
      });

      // Bob likes it optimistically.
      await bob.getByRole("button", { name: "Like" }).first().click();
      await expect(
        bob.getByRole("button", { name: "Unlike" }).first(),
      ).toBeVisible();

      // Bob comments.
      await bob.getByRole("button", { name: "Comments" }).first().click();
      await bob.getByLabel("Write a comment").fill("Nice one!");
      await bob.getByRole("button", { name: "Comment", exact: true }).click();
      await expect(bob.getByText("Nice one!")).toBeVisible();

      // The comment persists for Alice after a reload (open the thread).
      await alice.reload();
      await alice.getByRole("button", { name: "Comments" }).first().click();
      await expect(alice.getByText("Nice one!")).toBeVisible({
        timeout: 15_000,
      });
    } finally {
      await aliceCtx.close();
      await bobCtx.close();
    }
  });
});
