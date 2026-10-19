import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const h = vi.hoisted(() => {
  const ME = "11111111-1111-1111-1111-111111111111";
  const BOB = "22222222-2222-2222-2222-222222222222";
  const M1 = "33333333-3333-3333-3333-333333333333";
  return {
    ME,
    BOB,
    M1,
    people: vi.fn(),
    moments: vi.fn(),
    sendRequest: vi.fn(),
  };
});

vi.mock("@/features/auth/AuthProvider", () => ({
  useAuth: () => ({
    status: "authenticated",
    user: {
      id: h.ME,
      username: "me",
      email: "me@example.com",
      display_name: "Me",
      bio: null,
      avatar_url: null,
      status_message: null,
      online: true,
      last_seen_at: null,
      created_at: "2026-01-01T00:00:00Z",
    },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
    refresh: vi.fn(),
  }),
}));

vi.mock("@/features/discover/api", () => ({
  discoverApi: { people: h.people, moments: h.moments },
}));

vi.mock("@/features/contacts/api", () => ({
  contactsApi: { sendRequest: h.sendRequest },
}));

vi.mock("@/features/moments/api", () => ({
  momentsApi: {
    listComments: vi.fn(),
    addComment: vi.fn(),
    deleteComment: vi.fn(),
    like: vi.fn(),
    unlike: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    hide: vi.fn(),
    report: vi.fn(),
    listFeed: vi.fn(),
    create: vi.fn(),
    get: vi.fn(),
    uploadMedia: vi.fn(),
  },
}));

import { DiscoverView } from "@/features/discover/DiscoverView";

function profile(overrides: Record<string, unknown> = {}) {
  return {
    id: h.BOB,
    username: "bob",
    display_name: "Bob",
    avatar_url: null,
    status_message: null,
    online: false,
    last_seen_at: null,
    is_contact: false,
    ...overrides,
  };
}

function moment(overrides: Record<string, unknown> = {}) {
  return {
    id: h.M1,
    author: {
      id: h.BOB,
      username: "bob",
      display_name: "Bob",
      avatar_url: null,
    },
    body: "Public hello",
    visibility: "public",
    media: [],
    like_count: 0,
    comment_count: 0,
    liked_by_me: false,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderView() {
  return render(
    <ToastProvider>
      <DiscoverView />
    </ToastProvider>,
  );
}

describe("DiscoverView", () => {
  beforeEach(() => {
    h.people.mockReset();
    h.moments.mockReset();
    h.sendRequest.mockReset();

    h.people.mockResolvedValue({ items: [profile()], next_cursor: null });
    h.moments.mockResolvedValue({ items: [moment()], next_cursor: null });
  });

  it("loads people suggestions and public moments", async () => {
    renderView();

    await waitFor(() =>
      expect(screen.getByTestId("discover-people-results")).toBeInTheDocument(),
    );
    expect(
      within(screen.getByTestId("discover-people-results")).getByText("Bob"),
    ).toBeInTheDocument();
    expect(await screen.findByText("Public hello")).toBeInTheDocument();
    expect(h.people).toHaveBeenCalledTimes(1);
    expect(h.moments).toHaveBeenCalledTimes(1);
  });

  it("sends a friend request from a suggestion", async () => {
    h.sendRequest.mockResolvedValue({ status: "pending" });
    const user = userEvent.setup();
    renderView();

    await waitFor(() =>
      expect(screen.getByTestId("discover-people-results")).toBeInTheDocument(),
    );
    await user.click(screen.getByRole("button", { name: /add friend/i }));
    await waitFor(() => expect(h.sendRequest).toHaveBeenCalledWith(h.BOB));
  });

  it("shows an empty state when there are no suggestions", async () => {
    h.people.mockResolvedValue({ items: [], next_cursor: null });
    renderView();

    expect(
      await screen.findByText(/no suggestions right now/i),
    ).toBeInTheDocument();
  });

  it("shows an empty state when there are no public moments", async () => {
    h.moments.mockResolvedValue({ items: [], next_cursor: null });
    renderView();

    expect(
      await screen.findByText(/no public moments yet/i),
    ).toBeInTheDocument();
  });

  it("surfaces a retryable error when people fail to load", async () => {
    h.people.mockRejectedValue(new Error("boom"));
    renderView();

    expect(await screen.findByText(/could not load people/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
  });
});
