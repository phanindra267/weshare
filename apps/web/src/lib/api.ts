import { env } from "@/lib/env";
import type { ApiErrorBody, ApiDataBody } from "@/types/api";

/** Error thrown for any non-2xx API response or transport failure. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly details?: unknown;

  constructor(params: {
    status: number;
    code: string;
    message: string;
    requestId?: string;
    details?: unknown;
  }) {
    super(params.message);
    this.name = "ApiError";
    this.status = params.status;
    this.code = params.code;
    this.requestId = params.requestId;
    this.details = params.details;
  }

  /** True when the failure is a connectivity/transport problem (retryable). */
  get isNetworkError(): boolean {
    return this.status === 0;
  }
}

export interface RequestOptions extends Omit<RequestInit, "body"> {
  /** JSON-serializable body. Mutually exclusive with `formData`. */
  json?: unknown;
  /** Multipart body. Mutually exclusive with `json`. */
  formData?: FormData;
  /** Abort signal for cancellation. */
  signal?: AbortSignal;
}

interface Envelope<T> {
  data?: T;
  error?: ApiErrorBody["error"];
}

const UNSAFE_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);

/**
 * Read a cookie value by name. Returns undefined on the server (no document)
 * or when the cookie is absent.
 */
export function readCookie(name: string): string | undefined {
  if (typeof document === "undefined") return undefined;
  const prefix = `${name}=`;
  for (const part of document.cookie ? document.cookie.split("; ") : []) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length));
    }
  }
  return undefined;
}

/**
 * Perform an authenticated-ish JSON request against the REST API.
 *
 * Cookies are sent with `credentials: "include"` so HTTP-only session
 * cookies (introduced in Phase 1) work without storing secrets in JS. For
 * state-changing methods the double-submit CSRF token is echoed from its
 * cookie into the `X-CSRF-Token` header.
 */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { json, formData, headers, ...rest } = options;
  const url = path.startsWith("http")
    ? path
    : `${env.apiBaseUrl}${path.startsWith("/") ? path : `/${path}`}`;

  const finalHeaders = new Headers(headers);
  finalHeaders.set("Accept", "application/json");

  let body: BodyInit | undefined;
  if (json !== undefined && formData !== undefined) {
    throw new Error("Provide either `json` or `formData`, not both.");
  }
  if (json !== undefined) {
    finalHeaders.set("Content-Type", "application/json");
    body = JSON.stringify(json);
  } else if (formData !== undefined) {
    body = formData;
  }

  const method = (rest.method ?? "GET").toUpperCase();
  if (UNSAFE_METHODS.has(method) && !finalHeaders.has("X-CSRF-Token")) {
    const csrf = readCookie(env.csrfCookieName);
    if (csrf) finalHeaders.set("X-CSRF-Token", csrf);
  }

  let response: Response;
  try {
    response = await fetch(url, {
      ...rest,
      headers: finalHeaders,
      body,
      credentials: "include",
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new ApiError({
      status: 0,
      code: "NETWORK_ERROR",
      message:
        "Unable to reach the server. Check your connection and try again.",
      details: cause,
    });
  }

  const requestId = response.headers.get("X-Request-Id") ?? undefined;

  if (response.status === 204) {
    return undefined as T;
  }

  let payload: unknown = undefined;
  const text = await response.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = undefined;
    }
  }

  if (!response.ok) {
    const errBody =
      payload && typeof payload === "object"
        ? (payload as Envelope<T>).error
        : undefined;
    throw new ApiError({
      status: response.status,
      code: errBody?.code ?? "HTTP_ERROR",
      message:
        errBody?.message ?? `Request failed with status ${response.status}.`,
      requestId: errBody?.request_id ?? requestId,
      details: errBody?.details,
    });
  }

  if (payload && typeof payload === "object" && "data" in payload) {
    return (payload as ApiDataBody<T>).data;
  }
  return payload as T;
}

export const api = {
  get: <T>(path: string, options?: RequestOptions) =>
    apiRequest<T>(path, { ...options, method: "GET" }),
  post: <T>(path: string, options?: RequestOptions) =>
    apiRequest<T>(path, { ...options, method: "POST" }),
  patch: <T>(path: string, options?: RequestOptions) =>
    apiRequest<T>(path, { ...options, method: "PATCH" }),
  put: <T>(path: string, options?: RequestOptions) =>
    apiRequest<T>(path, { ...options, method: "PUT" }),
  delete: <T>(path: string, options?: RequestOptions) =>
    apiRequest<T>(path, { ...options, method: "DELETE" }),
};
