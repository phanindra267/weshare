import { env } from "@/lib/env";

/**
 * Resolve a server-relative media path (e.g. an avatar URL returned by the
 * API) into an absolute URL. Returns undefined for empty input.
 */
export function mediaUrl(path: string | null | undefined): string | undefined {
  if (!path) return undefined;
  if (path.startsWith("http://") || path.startsWith("https://")) return path;
  return `${env.apiBaseUrl}${path.startsWith("/") ? path : `/${path}`}`;
}
