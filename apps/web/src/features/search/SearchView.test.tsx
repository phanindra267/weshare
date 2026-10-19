import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const h = vi.hoisted(() => {
  const ME = "11111111-1111-1111-1111-111111111111";
  const BOB = "22222222-2222-2222-2222-222222222222";
  return {
    ME,
    BOB,
    messages: vi.fn(),
    users: vi.fn(),
    conversations: vi.fn(),
    posts: vi.fn(),
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

vi.mock("@/features/search/api", () => ({
  searchApi: {
    messages: h.messages,
    users: h.users,
    conversations: h.conversations,
    posts: h.posts,
  },
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

import { SearchView } from "@/features/search/SearchView";

const peer = {
  id: h.BOB,
  username: "bob",
  display_name: "Bob",
  avatar_url: null,
};

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

function chat(overrides: Record<string, unknown> = {}) {
  return {
    id: "c1",
    kind: "direct",
    title: null,
    peer,
    member_count: 2,
    role: "member",
    last_activity_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function messageHit() {
  return {
    message: {
      id: "m1",
      conversation_id: "c1",
      sender_id: h.BOB,
      sequence_no: 1,
      client_msg_id: "x",
      message_type: "text",
      content: { text: "hello bob" },
      created_at: "2026-01-01T00:00:00Z",
      edited_at: null,
      deleted_at: null,
    },
    conversation: { id: "c1", kind: "direct", title: null, peer },
  };
}

function renderView() {
  return render(
    <ToastProvider>
      <SearchView />
    </ToastProvider>,
  );
}

describe("SearchView", () => {
  beforeEach(() => {
    h.messages.mockReset();
    h.users.mockReset();
    h.conversations.mockReset();
    h.posts.mockReset();
    h.sendRequest.mockReset();

    h.messages.mockResolvedValue({ items: [messageHit()], next_cursor: null });
    h.users.mockResolvedValue({ items: [profile()], next_cursor: null });
    h.conversations.mockResolvedValue({ items: [chat()], next_cursor: null });
    h.posts.mockResolvedValue({ items: [], next_cursor: null });
  });

  it("shows the idle prompt and does not search before a query", () => {
    renderView();
    expect(screen.getByText(/start typing to search/i)).toBeInTheDocument();
    expect(h.users).not.toHaveBeenCalled();
    expect(h.messages).not.toHaveBeenCalled();
  });

  it("runs a global search across every domain on the All tab", async () => {
    const user = userEvent.setup();
    renderView();

    await user.type(screen.getByTestId("search-input"), "bob");

    await waitFor(() =>
      expect(screen.getByTestId("people-results")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("chat-results")).toBeInTheDocument();
    expect(screen.getByTestId("message-results")).toBeInTheDocument();
    expect(h.users).toHaveBeenCalledTimes(1);
    expect(h.conversations).toHaveBeenCalledTimes(1);
    expect(h.messages).toHaveBeenCalledTimes(1);
    expect(h.posts).toHaveBeenCalledTimes(1);
  });

  it("searches only people on the People tab", async () => {
    const user = userEvent.setup();
    renderView();

    await user.click(screen.getByRole("tab", { name: "People" }));
    await user.type(screen.getByTestId("search-input"), "bob");

    await waitFor(() =>
      expect(screen.getByTestId("people-results")).toBeInTheDocument(),
    );
    expect(h.users).toHaveBeenCalled();
    expect(h.messages).not.toHaveBeenCalled();
    expect(h.conversations).not.toHaveBeenCalled();
  });

  it("sends a friend request from a people result", async () => {
    h.sendRequest.mockResolvedValue({ status: "pending" });
    const user = userEvent.setup();
    renderView();

    await user.click(screen.getByRole("tab", { name: "People" }));
    await user.type(screen.getByTestId("search-input"), "bob");
    await waitFor(() =>
      expect(screen.getByTestId("people-results")).toBeInTheDocument(),
    );

    await user.click(screen.getByRole("button", { name: /add friend/i }));
    await waitFor(() => expect(h.sendRequest).toHaveBeenCalledWith(h.BOB));
  });

  it("reports an error when a search fails", async () => {
    h.users.mockRejectedValue(new Error("boom"));
    h.conversations.mockRejectedValue(new Error("boom"));
    h.messages.mockRejectedValue(new Error("boom"));
    h.posts.mockRejectedValue(new Error("boom"));
    const user = userEvent.setup();
    renderView();

    await user.type(screen.getByTestId("search-input"), "bob");

    await waitFor(() =>
      expect(screen.getByText(/search failed\. try again\./i)).toBeInTheDocument(),
    );
  });
});
