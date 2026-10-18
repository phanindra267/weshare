import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for end-to-end browser scenarios.
 *
 * Browser binaries are not bundled; run `npx playwright install chromium`
 * once before executing `npm run e2e`. See docs/TESTING.md.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  reporter: "list",
  globalSetup: "./e2e/global-setup.ts",
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000",
    trace: "on-first-retry",
    // Calls (Phase 9) need a camera and microphone. Feed Chromium synthetic
    // devices so WebRTC negotiates real tracks without any hardware, and
    // auto-grant the permission prompt it would otherwise raise.
    launchOptions: {
      args: [
        "--use-fake-device-for-media-stream",
        "--use-fake-ui-for-media-stream",
        "--autoplay-policy=no-user-gesture-required",
      ],
    },
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "npm run dev",
    url: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
