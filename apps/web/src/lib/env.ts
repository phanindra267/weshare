/**
 * Public (browser-visible) environment configuration.
 *
 * Only variables prefixed with `NEXT_PUBLIC_` are available on the client.
 * Values are validated at module load so misconfiguration fails fast and
 * visibly rather than surfacing as a confusing runtime error later.
 */

function readUrl(name: string, fallback: string, protocols: string[]): string {
  const raw = process.env[name] ?? fallback;
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new Error(
      `Invalid ${name}="${raw}". Expected an absolute URL such as ${fallback}.`,
    );
  }
  if (!protocols.includes(parsed.protocol)) {
    throw new Error(
      `Invalid ${name}="${raw}". Expected protocol ${protocols.join(" or ")}.`,
    );
  }
  return parsed.toString().replace(/\/$/, "");
}

const DEFAULT_API_ORIGIN = "http://localhost:8080";

export const env = {
  /** REST API base, e.g. http://localhost:8080 (no trailing slash). */
  apiBaseUrl: readUrl("NEXT_PUBLIC_API_BASE_URL", DEFAULT_API_ORIGIN, [
    "http:",
    "https:",
  ]),
  /** WebSocket origin, e.g. ws://localhost:8080 (no trailing slash). */
  wsBaseUrl: readUrl(
    "NEXT_PUBLIC_WS_URL",
    DEFAULT_API_ORIGIN.replace(/^http/, "ws"),
    ["ws:", "wss:"],
  ),
  /**
   * Name of the JavaScript-readable CSRF cookie the API sets. The client
   * echoes its value in the `X-CSRF-Token` header on state-changing requests.
   */
  csrfCookieName: process.env.NEXT_PUBLIC_CSRF_COOKIE_NAME ?? "weshare_csrf",
} as const;
