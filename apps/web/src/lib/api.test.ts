import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { apiRequest, ApiError } from "@/lib/api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-Id": "req-1" },
  });
}

function clearCookies() {
  for (const c of document.cookie.split("; ")) {
    const name = c.split("=")[0];
    if (name) {
      document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
    }
  }
}

describe("apiRequest", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    clearCookies();
    fetchMock = vi
      .fn()
      .mockImplementation(() => Promise.resolve(jsonResponse({ ok: true })));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("unwraps the data envelope", async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(jsonResponse({ data: { id: "42" } })),
    );
    const result = await apiRequest<{ id: string }>("/api/v1/thing");
    expect(result).toEqual({ id: "42" });
  });

  it("echoes the CSRF cookie on unsafe methods", async () => {
    document.cookie = "weshare_csrf=token-abc; path=/";
    await apiRequest("/api/v1/auth/logout", { method: "POST" });

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    const headers = new Headers(init.headers);
    expect(headers.get("X-CSRF-Token")).toBe("token-abc");
  });

  it("does not send a CSRF header on safe methods", async () => {
    document.cookie = "weshare_csrf=token-abc; path=/";
    await apiRequest("/api/v1/auth/me", { method: "GET" });

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    const headers = new Headers(init.headers);
    expect(headers.get("X-CSRF-Token")).toBeNull();
  });

  it("throws a typed error from the error envelope", async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        jsonResponse(
          {
            error: {
              code: "UNAUTHORIZED",
              message: "Invalid email or password.",
              request_id: "req-9",
            },
          },
          401,
        ),
      ),
    );

    await expect(
      apiRequest("/api/v1/auth/login", { method: "POST", json: {} }),
    ).rejects.toMatchObject({
      status: 401,
      code: "UNAUTHORIZED",
      message: "Invalid email or password.",
    });
    await expect(
      apiRequest("/api/v1/auth/login", { method: "POST", json: {} }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});
