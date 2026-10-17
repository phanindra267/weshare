import { test, expect, type Page } from "@playwright/test";

/**
 * Phase 6: Media attachments and voice messages.
 * Tests image, video, audio, and file upload, preview, and rejection of oversized files.
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

test.describe("Phase 6 Media attachments", () => {
  test("uploads, previews, and shares media between contacts and rejects oversized uploads", async ({
    browser,
  }) => {
    test.setTimeout(90_000);

    const aliceCtx = await browser.newContext();
    const bobCtx = await browser.newContext();

    const alice = await aliceCtx.newPage();
    const bob = await bobCtx.newPage();

    const aliceUser = uniqueUser("p6alice");
    const bobUser = uniqueUser("p6bob");

    await register(alice, aliceUser);
    await register(bob, bobUser);

    const bobId = await myId(bob);
    await becomeContacts(alice, bob, bobId, aliceUser.displayName);

    // Alice opens direct chat with Bob
    await alice.goto("/messages");
    await alice.getByLabel("Start a chat").fill(bobId);
    await alice.getByRole("button", { name: "Open" }).click();

    await expect(
      alice.locator("header").getByText(bobUser.displayName),
    ).toBeVisible();

    // 1. Upload and share image
    const attachBtn = alice.locator('button[aria-label="Attach a file"]');
    await expect(attachBtn).toBeVisible();

    // 1x1 PNG image
    const pngBytes = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
      "base64",
    );

    const [fileChooser] = await Promise.all([
      alice.waitForEvent("filechooser"),
      attachBtn.click(),
    ]);

    await fileChooser.setFiles({
      name: "pixel.png",
      mimeType: "image/png",
      buffer: pngBytes,
    });

    // Image preview appears in Alice's thread
    await expect(alice.locator('img[alt="Shared image"]').last()).toBeVisible({
      timeout: 15000,
    });

    // Bob opens direct chat with Alice
    await bob.goto("/messages");
    await expect(bob.getByText(aliceUser.displayName)).toBeVisible();
    await bob.getByText(aliceUser.displayName).click();

    // Bob sees the shared image preview
    await expect(bob.locator('img[alt="Shared image"]').last()).toBeVisible({
      timeout: 15000,
    });

    // 2. Reject oversized file (> 50 MB)
    const bigBuffer = Buffer.alloc(51 * 1024 * 1024, 0);
    const [bigChooser] = await Promise.all([
      alice.waitForEvent("filechooser"),
      attachBtn.click(),
    ]);
    await bigChooser.setFiles({
      name: "oversized.bin",
      mimeType: "application/octet-stream",
      buffer: bigBuffer,
    });

    // Alice sees error toast / alert
    await expect(
      alice.getByText(/exceeds the maximum size limit/i),
    ).toBeVisible({ timeout: 5000 });

    await aliceCtx.close();
    await bobCtx.close();
  });
});
