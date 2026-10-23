import { test, expect, type Page } from "@playwright/test";

/**
 * Live end-to-end advanced messaging (Phase 4): edit, delete, reply, forward,
 * react, pin, and in-chat search between two real browser sessions.
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
  await expect(bob.getByText(aliceName)).toBeVisible();
}

/** The chat panel, scoped away from the conversation list in the sidebar. */
function thread(page: Page) {
  return page
    .getByLabel("Message", { exact: true })
    .locator("xpath=ancestor::section[1]");
}

/** A conversation entry in the sidebar (the chat header has call buttons too). */
function conversationEntry(page: Page, name: string) {
  return page.locator("aside").getByRole("button", { name }).first();
}

/** The list item for the message bubble containing `text`. */
function bubble(page: Page, text: string) {
  return thread(page).getByText(text, { exact: true }).first();
}

async function sendMessage(page: Page, text: string) {
  await page.getByLabel("Message", { exact: true }).fill(text);
  await page.getByRole("button", { name: "Send", exact: true }).click();
}

test.describe("advanced messaging end to end", () => {
  test.describe.configure({ timeout: 180_000 });

  test("edit, react, pin, reply, forward, search, and delete all work", async ({
    browser,
  }) => {
    const aliceContext = await browser.newContext();
    const bobContext = await browser.newContext();
    const alice = await aliceContext.newPage();
    const bob = await bobContext.newPage();
    // Surface actionability problems quickly instead of burning the whole
    // test timeout on a single stuck locator.
    alice.setDefaultTimeout(15_000);
    bob.setDefaultTimeout(15_000);

    try {
      const aliceUser = uniqueUser("alice");
      const bobUser = uniqueUser("bob");

      await register(alice, aliceUser);
      await register(bob, bobUser);

      const bobId = await myId(bob);
      await becomeContacts(alice, bob, bobId, aliceUser.displayName);

      // Alice opens a chat with Bob.
      await alice.goto("/messages");
      await alice.getByLabel("Start a chat").fill(bobId);
      await alice.getByRole("button", { name: "Open", exact: true }).click();
      await expect(alice.getByLabel("Message", { exact: true })).toBeVisible();

      // Bob opens the same thread.
      await bob.goto("/messages");
      await conversationEntry(bob, aliceUser.displayName).click();
      await expect(bob.getByLabel("Message", { exact: true })).toBeVisible();

      // --- send ---------------------------------------------------------
      await sendMessage(alice, "Original message");
      await expect(bubble(alice, "Original message")).toBeVisible();
      await expect(bubble(bob, "Original message")).toBeVisible({
        timeout: 10_000,
      });

      // --- edit ---------------------------------------------------------
      const own = bubble(alice, "Original message").locator("xpath=ancestor::li[1]");
      await own.hover();
      await own.getByRole("button", { name: "Edit" }).click();

      const composer = alice.getByLabel("Message", { exact: true });
      await expect(composer).toHaveValue("Original message");
      await expect(alice.getByText("Editing message")).toBeVisible();
      await composer.fill("Edited message");
      await alice.getByRole("button", { name: "Save", exact: true }).click();

      await expect(bubble(alice, "Edited message")).toBeVisible();
      await expect(thread(alice).getByText("(edited)")).toBeVisible();
      // The edit reaches Bob over the realtime socket, without a reload.
      await expect(bubble(bob, "Edited message")).toBeVisible({
        timeout: 10_000,
      });

      // --- react --------------------------------------------------------
      const edited = bubble(alice, "Edited message").locator("xpath=ancestor::li[1]");
      await edited.hover();
      await edited.getByRole("button", { name: "Add reaction" }).click();
      await edited.getByRole("button", { name: "React with 👍" }).click();

      await expect(
        thread(alice).getByLabel("👍 reaction, 1"),
      ).toBeVisible();
      // The reaction reaches Bob live, with his own viewer-specific aggregate.
      await expect(thread(bob).getByLabel("👍 reaction, 1")).toBeVisible({
        timeout: 10_000,
      });

      // --- pin ----------------------------------------------------------
      await edited.hover();
      await edited.getByRole("button", { name: "Pin", exact: true }).click();
      await expect(thread(alice).getByLabel("Pinned").first()).toBeVisible();
      await expect(thread(bob).getByLabel("Pinned").first()).toBeVisible({
        timeout: 10_000,
      });

      // --- reply --------------------------------------------------------
      // Bob replies to Alice's (edited) message.
      const aliceEdited = bubble(bob, "Edited message").locator(
        "xpath=ancestor::li[1]",
      );
      await aliceEdited.hover();
      await aliceEdited.getByRole("button", { name: "Reply" }).click();
      await expect(bob.getByText("Replying to")).toBeVisible();
      await sendMessage(bob, "A reply from Bob");

      // Alice sees the reply, with the quoted message, live.
      const reply = bubble(alice, "A reply from Bob").locator(
        "xpath=ancestor::li[1]",
      );
      await expect(reply).toBeVisible({ timeout: 10_000 });
      await expect(reply.getByText("Edited message")).toBeVisible();

      // --- forward ------------------------------------------------------
      // Alice needs a second conversation: a group with Bob.
      await alice.getByRole("button", { name: "New group" }).click();
      await alice.getByLabel("Group name").fill("Forward crew");
      await alice.getByRole("checkbox").first().check();
      await alice.getByRole("button", { name: "Create group" }).click();
      await expect(thread(alice).getByText("Forward crew")).toBeVisible();

      // Back to the direct chat, then forward a message into the group.
      await conversationEntry(alice, bobUser.displayName).click();
      const toForward = bubble(alice, "Edited message").locator(
        "xpath=ancestor::li[1]",
      );
      await toForward.hover();
      await toForward.getByRole("button", { name: "Forward" }).click();

      const forwardDialog = alice.getByRole("dialog");
      await expect(forwardDialog).toBeVisible();
      await forwardDialog.getByRole("button", { name: /Forward crew/ }).click();

      // The forwarded copy lands in the group, labelled as forwarded.
      await conversationEntry(alice, "Forward crew").click();
      await expect(thread(alice).getByText("Forwarded")).toBeVisible({
        timeout: 10_000,
      });
      await expect(
        thread(alice).getByText("Edited message").first(),
      ).toBeVisible();

      // --- search -------------------------------------------------------
      await conversationEntry(alice, "Forward crew").click();
      await alice.getByRole("button", { name: "Search", exact: true }).click();
      await alice
        .getByLabel("Search messages in this conversation")
        .fill("Edited");
      await alice.getByRole("button", { name: "Search messages" }).click();
      await expect(
        thread(alice).getByRole("button", { name: /Edited message/ }),
      ).toBeVisible();

      // --- delete -------------------------------------------------------
      // Delete the message in the direct chat; both sides show a tombstone.
      await conversationEntry(alice, bobUser.displayName).click();
      const toDelete = bubble(alice, "Edited message").locator(
        "xpath=ancestor::li[1]",
      );
      await toDelete.hover();
      await toDelete.getByRole("button", { name: "Delete" }).click();

      const deleteDialog = alice.getByRole("dialog");
      await expect(deleteDialog).toBeVisible();
      await deleteDialog.getByRole("button", { name: "Delete" }).click();

      await expect(
        thread(alice).getByText("This message was deleted."),
      ).toBeVisible();
      // Bob sees the tombstone live.
      await expect(
        thread(bob).getByText("This message was deleted."),
      ).toBeVisible({ timeout: 10_000 });
    } finally {
      await aliceContext.close();
      await bobContext.close();
    }
  });
});
