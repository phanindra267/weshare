import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@/lib/api";

const { pushMock, loginMock } = vi.hoisted(() => ({
  pushMock: vi.fn(),
  loginMock: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/login",
}));

vi.mock("@/features/auth/AuthProvider", () => ({
  useAuth: () => ({
    status: "anonymous",
    user: null,
    login: loginMock,
    register: vi.fn(),
    logout: vi.fn(),
    refresh: vi.fn(),
  }),
}));

import { LoginForm } from "@/features/auth/LoginForm";

describe("LoginForm", () => {
  beforeEach(() => {
    pushMock.mockReset();
    loginMock.mockReset();
  });

  it("submits credentials and redirects to the profile", async () => {
    loginMock.mockResolvedValue({ id: "1" });
    render(<LoginForm />);

    await userEvent.type(screen.getByLabelText(/email/i), "alice@example.com");
    await userEvent.type(screen.getByLabelText(/password/i), "supersecret1");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => {
      expect(loginMock).toHaveBeenCalledWith({
        email: "alice@example.com",
        password: "supersecret1",
      });
    });
    expect(pushMock).toHaveBeenCalledWith("/profile");
  });

  it("shows a generic message for invalid credentials", async () => {
    loginMock.mockRejectedValue(
      new ApiError({
        status: 401,
        code: "UNAUTHORIZED",
        message: "Invalid email or password.",
      }),
    );
    render(<LoginForm />);

    await userEvent.type(screen.getByLabelText(/email/i), "alice@example.com");
    await userEvent.type(screen.getByLabelText(/password/i), "wrong");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(
      await screen.findByText(/invalid email or password/i),
    ).toBeInTheDocument();
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("renders per-field validation errors", async () => {
    loginMock.mockRejectedValue(
      new ApiError({
        status: 422,
        code: "VALIDATION_ERROR",
        message: "Please fix the highlighted fields.",
        details: { fields: { email: "Email is required." } },
      }),
    );
    render(<LoginForm />);

    await userEvent.type(screen.getByLabelText(/email/i), "a@b.com");
    await userEvent.type(screen.getByLabelText(/password/i), "x");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByText("Email is required.")).toBeInTheDocument();
  });
});
