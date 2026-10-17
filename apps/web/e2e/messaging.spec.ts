import { test, expect, type Page } from "@playwright/test";

/**
 * Live end-to-end messaging: two users become contacts, open a direct chat,
 * and exchange messages over the realtime WebSocket in real time.
 *
 * Requires the API on http://localhost:8080 (see docs/TESTING.md). The Next dev
 * server is started by the Playwright `webServer` config.
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

async function becomeContacts(
  alice: Page,
  bob: Page,
  bobId: string,
  aliceName: string,
) {
  await alice.goto("/contacts");
  await alice.getByLabel("User id").fill(bobId);
  await alice.getByRole("button", { name: "Send request" }).click();

  await bob.goto("/contacts");
  await bob.getByRole("button", { name: "Accept" }).click();
  // After accepting, Bob's contacts list includes Alice.
  await expect(bob.getByText(aliceName)).toBeVisible();
}

test.describe("messaging end to end", () => {
  test.describe.configure({ timeout: 120_000 });

  test("two connected users chat live over the realtime socket", async ({
    browser,
  }) => {
    const aliceContext = await browser.newContext();
    const bobContext = await browser.newContext();
    const alice = await aliceContext.newPage();
    const bob = await bobContext.newPage();

    try {
      const aliceUser = uniqueUser("alice");
      const bobUser = uniqueUser("bob");

      await register(alice, aliceUser);
      await register(bob, bobUser);

      const bobId = await myId(bob);
      await becomeContacts(alice, bob, bobId, aliceUser.displayName);

      // Alice starts a chat with Bob.
      await alice.goto("/messages");
      await alice.getByLabel("Start a chat").fill(bobId);
      await alice.getByRole("button", { name: "Open", exact: true }).click();
      await expect(alice.getByLabel("Message", { exact: true })).toBeVisible();

      // Presence: both browsers are connected, so Bob shows online to Alice.
      await expect(alice.locator('[title="Online"]').first()).toBeVisible({
        timeout: 10_000,
      });

      // Alice sends the first message.
      await alice
        .getByLabel("Message", { exact: true })
        .fill("Hello from Alice");
      await alice.getByRole("button", { name: "Send" }).click();
      await expect(alice.getByText("Hello from Alice").first()).toBeVisible();

      // Bob sees it in his conversation list and opens the thread.
      await bob.goto("/messages");
      await bob.getByRole("button", { name: aliceUser.displayName }).click();
      await expect(bob.getByText("Hello from Alice").first()).toBeVisible();

      // Bob replies; Alice receives it live without reloading.
      await bob.getByLabel("Message", { exact: true }).fill("Hi Alice, live!");
      await bob.getByRole("button", { name: "Send" }).click();
      await expect(alice.getByText("Hi Alice, live!").first()).toBeVisible({
        timeout: 10_000,
      });

      // Bob has read the thread, so Alice's own message ticks to Read.
      await expect(alice.getByLabel("Read").first()).toBeVisible({
        timeout: 10_000,
      });
    } finally {
      await aliceContext.close();
      await bobContext.close();
    }
  });
});
