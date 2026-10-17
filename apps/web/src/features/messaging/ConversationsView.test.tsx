import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const h = vi.hoisted(() => {
  const ME = "11111111-1111-1111-1111-111111111111";
  const BOB = "22222222-2222-2222-2222-222222222222";
  const CONV = "33333333-3333-3333-3333-333333333333";
  const listeners = new Set<(event: unknown) => void>();
  const send = vi.fn((event: unknown) => {
    void event;
    return true;
  });
  const subscribe = vi.fn((listener: (event: unknown) => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  });
  return {
    ME,
    BOB,
    CONV,
    listeners,
    send,
    subscribe,
    listConversations: vi.fn(),
    listMessages: vi.fn(),
    sendMessage: vi.fn(),
    markRead: vi.fn(),
    openConversation: vi.fn(),
  };
});

vi.mock("@/features/messaging/RealtimeProvider", () => ({
  useRealtime: () => ({
    status: "open",
    connectionId: 1,
    send: h.send,
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

vi.mock("@/features/messaging/api", () => ({
  messagingApi: {
    listConversations: h.listConversations,
    listMessages: h.listMessages,
    sendMessage: h.sendMessage,
    markRead: h.markRead,
    openConversation: h.openConversation,
    getConversation: vi.fn(),
  },
}));

import { ConversationsView } from "@/features/messaging/ConversationsView";

const CONVERSATION = {
  id: h.CONV,
  kind: "direct" as const,
  peer: {
    id: h.BOB,
    username: "bob",
    display_name: "Bob",
    avatar_url: null,
    status_message: null,
    online: true,
    last_seen_at: null,
  },
  last_message: null,
  unread_count: 0,
  created_at: "2026-01-01T00:00:00Z",
};

function incomingMessage(overrides: Record<string, unknown> = {}) {
  return {
    id: "m1",
    conversation_id: h.CONV,
    sender_id: h.BOB,
    sequence_no: 1,
    client_msg_id: null,
    message_type: "text",
    content: { text: "Hello there" },
    status: "received",
    created_at: "2026-01-01T00:00:00Z",
    edited_at: null,
    deleted_at: null,
    ...overrides,
  };
}

function emit(event: unknown) {
  h.listeners.forEach((listener) => listener(event));
}

function renderView() {
  return render(
    <ToastProvider>
      <ConversationsView />
    </ToastProvider>,
  );
}

describe("ConversationsView", () => {
  beforeEach(() => {
    h.listeners.clear();
    h.send.mockClear();
    h.subscribe.mockClear();
    h.listConversations.mockReset();
    h.listMessages.mockReset();
    h.sendMessage.mockReset();
    h.markRead.mockReset();
    h.openConversation.mockReset();

    Element.prototype.scrollIntoView = vi.fn();

    h.listConversations.mockResolvedValue({
      items: [CONVERSATION],
      next_cursor: "",
    });
    h.listMessages.mockResolvedValue({
      items: [incomingMessage()],
      next_cursor: "",
    });
    h.markRead.mockResolvedValue({ unread_count: 0 });
  });

  it("lists conversations from the API", async () => {
    renderView();
    expect(await screen.findByText("Bob")).toBeInTheDocument();
  });

  it("loads and displays history when a conversation is selected", async () => {
    renderView();
    await userEvent.click(await screen.findByText("Bob"));
    expect(await screen.findByText("Hello there")).toBeInTheDocument();
    await waitFor(() => {
      expect(h.listMessages).toHaveBeenCalledWith(
        h.CONV,
        expect.objectContaining({ limit: expect.any(Number) }),
      );
    });
  });

  it("sends an optimistic message over the socket", async () => {
    renderView();
    await userEvent.click(await screen.findByText("Bob"));
    await screen.findByText("Hello there");

    await userEvent.type(screen.getByLabelText("Message"), "Hi Bob");
    await userEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(screen.getByText("Hi Bob")).toBeInTheDocument();
    expect(h.send).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "message.send",
        conversation_id: h.CONV,
        content: { text: "Hi Bob" },
      }),
    );
  });

  it("reconciles an optimistic message with the server ack", async () => {
    renderView();
    await userEvent.click(await screen.findByText("Bob"));
    await screen.findByText("Hello there");

    await userEvent.type(screen.getByLabelText("Message"), "Hi Bob");
    await userEvent.click(screen.getByRole("button", { name: /send/i }));

    const sent = h.send.mock.calls.at(-1)?.[0] as { client_msg_id: string };

    emit({
      type: "message.ack",
      conversation_id: h.CONV,
      message: incomingMessage({
        id: "server-sent",
        sender_id: h.ME,
        sequence_no: 2,
        client_msg_id: sent.client_msg_id,
        content: { text: "Hi Bob" },
        status: "sent",
      }),
    });

    await waitFor(() => {
      expect(screen.getAllByText("Hi Bob")).toHaveLength(1);
    });
  });

  it("appends a live incoming message and shows typing", async () => {
    renderView();
    await userEvent.click(await screen.findByText("Bob"));
    await screen.findByText("Hello there");

    emit({
      type: "message.created",
      event_id: "e1",
      conversation_id: h.CONV,
      message: incomingMessage({
        id: "m2",
        sequence_no: 2,
        content: { text: "Are you there?" },
      }),
    });

    expect(
      (await screen.findAllByText("Are you there?")).length,
    ).toBeGreaterThan(0);

    emit({
      type: "typing",
      conversation_id: h.CONV,
      user_id: h.BOB,
      typing: true,
    });

    expect(await screen.findByText(/Bob is typing/)).toBeInTheDocument();
  });

  it("marks the sender's messages as read on a receipt", async () => {
    renderView();
    await userEvent.click(await screen.findByText("Bob"));
    await screen.findByText("Hello there");

    await userEvent.type(screen.getByLabelText("Message"), "Ping");
    await userEvent.click(screen.getByRole("button", { name: /send/i }));

    const sent = h.send.mock.calls.at(-1)?.[0] as { client_msg_id: string };
    emit({
      type: "message.ack",
      conversation_id: h.CONV,
      message: incomingMessage({
        id: "server-ping",
        sender_id: h.ME,
        sequence_no: 2,
        client_msg_id: sent.client_msg_id,
        content: { text: "Ping" },
        status: "sent",
      }),
    });

    emit({
      type: "message.receipt",
      conversation_id: h.CONV,
      user_id: h.BOB,
      status: "read",
      up_to_sequence: 2,
    });

    expect(await screen.findByLabelText("Read")).toBeInTheDocument();
  });
});
