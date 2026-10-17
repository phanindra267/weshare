import { request } from "@playwright/test";

/**
 * Pre-warm the Next dev server before the suite runs.
 *
 * In development Next compiles each route on first request, which can take tens
 * of seconds on a cold cache. Without this, the first test that touches a route
 * absorbs that cost and can blow its timeout. Requesting every route up front
 * moves compilation out of the tests. The API must already be running (see
 * docs/TESTING.md).
 */
const ROUTES = ["/", "/register", "/login", "/profile", "/contacts", "/messages"];

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";

export default async function globalSetup() {
  const ctx = await request.newContext({ baseURL: BASE_URL });
  try {
    for (const path of ROUTES) {
      try {
        await ctx.get(path, { timeout: 120_000, maxRedirects: 5 });
      } catch {
        // Best-effort: if warm-up fails the tests will surface the real error.
      }
    }
  } finally {
    await ctx.dispose();
  }
}
