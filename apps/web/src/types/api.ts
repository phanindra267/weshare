/**
 * Shared API contract types.
 *
 * These mirror the backend response envelope documented in
 * `packages/contracts/openapi.yaml`. Keep them in sync with the Go handlers.
 */

/** Standard error body returned by every failing `/api/v1` endpoint. */
export interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    request_id?: string;
    details?: unknown;
  };
}

/** Standard success wrapper for single resources. */
export interface ApiDataBody<T> {
  data: T;
}

/** Cursor-paginated list envelope. */
export interface Paginated<T> {
  items: T[];
  next_cursor: string | null;
}

export interface HealthResponse {
  status: "ok";
  service: string;
  version: string;
  uptime_seconds: number;
}

export interface ReadyResponse {
  status: "ready";
  checks: Record<string, "ok" | "error">;
}
