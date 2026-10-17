import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const h = vi.hoisted(() => {
  const ME = "11111111-1111-1111-1111-111111111111";
  const BOB = "22222222-2222-2222-2222-222222222222";
  const M1 = "33333333-3333-3333-3333-333333333333";
  const M2 = "44444444-4444-4444-4444-444444444444";
  const listeners = new Set<(event: unknown) => void>();
  const subscribe = vi.fn((listener: (event: unknown) => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  });
  return {
    ME,
    BOB,
    M1,
    M2,
    listeners,
    subscribe,
    listFeed: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    hide: vi.fn(),
    report: vi.fn(),
    like: vi.fn(),
    unlike: vi.fn(),
    listComments: vi.fn(),
    addComment: vi.fn(),
    deleteComment: vi.fn(),
    uploadMedia: vi.fn(),
  };
});

vi.mock("@/features/messaging/RealtimeProvider", () => ({
  useRealtime: () => ({
    status: "open",
    connectionId: 1,
    send: vi.fn(),
    subscribe: h.subscribe,
  }),
}));

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

vi.mock("@/features/moments/api", () => ({
  momentsApi: {
    listFeed: h.listFeed,
    create: h.create,
    update: h.update,
    remove: h.remove,
    hide: h.hide,
    report: h.report,
    like: h.like,
    unlike: h.unlike,
    listComments: h.listComments,
    addComment: h.addComment,
    deleteComment: h.deleteComment,
    uploadMedia: h.uploadMedia,
  },
}));

import { MomentsView } from "@/features/moments/MomentsView";

function moment(overrides: Record<string, unknown> = {}) {
  return {
    id: h.M1,
    author: {
      id: h.BOB,
      username: "bob",
      display_name: "Bob",
      avatar_url: null,
    },
    body: "Hello from Bob",
    visibility: "friends",
    media: [],
    like_count: 0,
    comment_count: 0,
    liked_by_me: false,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function emit(event: unknown) {
  h.listeners.forEach((listener) => listener(event));
}

function renderView() {
  return render(
    <ToastProvider>
      <MomentsView />
    </ToastProvider>,
  );
}

describe("MomentsView", () => {
  beforeEach(() => {
    h.listeners.clear();
    h.subscribe.mockClear();
    h.listFeed.mockReset();
    h.create.mockReset();
    h.update.mockReset();
    h.remove.mockReset();
    h.hide.mockReset();
    h.report.mockReset();
    h.like.mockReset();
    h.unlike.mockReset();
    h.listComments.mockReset();
    h.addComment.mockReset();
    h.deleteComment.mockReset();
    h.uploadMedia.mockReset();

    h.listFeed.mockResolvedValue({ items: [moment()], next_cursor: null });
    h.listComments.mockResolvedValue({ items: [], next_cursor: null });
  });

  it("lists moments from the API", async () => {
    renderView();
    expect(await screen.findByText("Hello from Bob")).toBeInTheDocument();
  });

  it("prepends a moment shared by a friend in realtime", async () => {
    renderView();
    await screen.findByText("Hello from Bob");

    emit({
      type: "moment.created",
      moment: moment({ id: h.M2, body: "Fresh moment", author: { id: h.BOB, username: "bob", display_name: "Bob", avatar_url: null } }),
    });

    expect(await screen.findByText("Fresh moment")).toBeInTheDocument();
  });

  it("likes a moment optimistically and reconciles the server count", async () => {
    h.like.mockResolvedValue({ like_count: 5, liked: true });
    renderView();
    await screen.findByText("Hello from Bob");

    await userEvent.click(screen.getByRole("button", { name: "Like" }));

    // Optimistic toggle happens immediately.
    expect(screen.getByRole("button", { name: "Unlike" })).toBeInTheDocument();
    expect(h.like).toHaveBeenCalledWith(h.M1);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Unlike" })).toHaveTextContent(
        "5",
      );
    });
  });

  it("shares a new moment through the composer", async () => {
    h.create.mockResolvedValue(
      moment({
        id: h.M2,
        body: "My new moment",
        author: { id: h.ME, username: "me", display_name: "Me", avatar_url: null },
      }),
    );
    renderView();
    await screen.findByText("Hello from Bob");

    await userEvent.type(screen.getByLabelText("Moment body"), "My new moment");
    await userEvent.click(screen.getByRole("button", { name: "Share" }));

    expect(await screen.findByText("My new moment")).toBeInTheDocument();
    expect(h.create).toHaveBeenCalledWith(
      expect.objectContaining({ body: "My new moment", visibility: "friends" }),
    );
  });

  it("adds a comment and bumps the comment count", async () => {
    h.addComment.mockResolvedValue({
      id: h.M2,
      moment_id: h.M1,
      author: { id: h.ME, username: "me", display_name: "Me", avatar_url: null },
      body: "Nice!",
      created_at: "2026-01-01T00:00:00Z",
    });
    renderView();
    await screen.findByText("Hello from Bob");

    await userEvent.click(screen.getByRole("button", { name: "Comments" }));
    await userEvent.type(screen.getByLabelText("Write a comment"), "Nice!");
    await userEvent.click(screen.getByRole("button", { name: /^Comment$/ }));

    expect(await screen.findByText("Nice!")).toBeInTheDocument();
    expect(h.addComment).toHaveBeenCalledWith(h.M1, "Nice!");
  });

  it("edits an authored moment", async () => {
    const myMoment = moment({
      id: h.M2,
      body: "My initial post",
      author: { id: h.ME, username: "me", display_name: "Me", avatar_url: null },
    });
    h.listFeed.mockResolvedValue({ items: [myMoment], next_cursor: null });
    h.update.mockResolvedValue({ ...myMoment, body: "My edited post" });

    renderView();
    expect(await screen.findByText("My initial post")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Edit moment" }));
    const editInput = screen.getByLabelText("Edit post body");
    await userEvent.clear(editInput);
    await userEvent.type(editInput, "My edited post");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("My edited post")).toBeInTheDocument();
    expect(h.update).toHaveBeenCalledWith(h.M2, {
      body: "My edited post",
      visibility: "friends",
    });
  });

  it("hides a moment from the feed", async () => {
    h.hide.mockResolvedValue(undefined);
    renderView();
    expect(await screen.findByText("Hello from Bob")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Hide moment" }));
    expect(h.hide).toHaveBeenCalledWith(h.M1);
    await waitFor(() => {
      expect(screen.queryByText("Hello from Bob")).not.toBeInTheDocument();
    });
  });

  it("reports a moment with reason", async () => {
    h.report.mockResolvedValue(undefined);
    renderView();
    expect(await screen.findByText("Hello from Bob")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Report moment" }));
    expect(screen.getByText("Report Post")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Submit Report" }));
    expect(h.report).toHaveBeenCalledWith(h.M1, {
      reason: "spam",
      details: undefined,
    });
  });

  it("replies to an existing comment", async () => {
    const parentComment = {
      id: "99999999-9999-9999-9999-999999999999",
      moment_id: h.M1,
      author: { id: h.BOB, username: "bob", display_name: "Bob", avatar_url: null },
      body: "Great post!",
      created_at: "2026-01-01T00:00:00Z",
    };
    h.listComments.mockResolvedValue({ items: [parentComment], next_cursor: null });
    h.addComment.mockResolvedValue({
      id: "88888888-8888-8888-8888-888888888888",
      moment_id: h.M1,
      author: { id: h.ME, username: "me", display_name: "Me", avatar_url: null },
      reply_to_id: parentComment.id,
      reply_to_author: parentComment.author,
      body: "Thanks Bob!",
      created_at: "2026-01-01T00:01:00Z",
    });

    renderView();
    await screen.findByText("Hello from Bob");

    await userEvent.click(screen.getByRole("button", { name: "Comments" }));
    expect(await screen.findByText("Great post!")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Reply to Bob" }));
    expect(screen.getByText(/Replying to/)).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Write a comment"), "Thanks Bob!");
    await userEvent.click(screen.getByRole("button", { name: /^Comment$/ }));

    expect(await screen.findByText("Thanks Bob!")).toBeInTheDocument();
    expect(h.addComment).toHaveBeenCalledWith(h.M1, "Thanks Bob!", parentComment.id);
  });
});
