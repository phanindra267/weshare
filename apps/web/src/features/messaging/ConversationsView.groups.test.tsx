import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/components/ui/Toast";

const h = vi.hoisted(() => {
  const ME = "11111111-1111-1111-1111-111111111111";
  const BOB = "22222222-2222-2222-2222-222222222222";
  const CAROL = "44444444-4444-4444-4444-444444444444";
  const CONV = "33333333-3333-3333-3333-333333333333";
  const GROUP = "55555555-5555-5555-5555-555555555555";
  const listeners = new Set<(event: unknown) => void>();
  const send = vi.fn(() => true);
  const subscribe = vi.fn((listener: (event: unknown) => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  });
  return {
    ME,
    BOB,
    CAROL,
    CONV,
    GROUP,
    listeners,
    send,
    subscribe,
    listConversations: vi.fn(),
    listMessages: vi.fn(),
    sendMessage: vi.fn(),
    markRead: vi.fn(),
    openConversation: vi.fn(),
    getConversation: vi.fn(),
    createGroup: vi.fn(),
    updateGroup: vi.fn(),
    listMembers: vi.fn(),
    addMembers: vi.fn(),
    removeMember: vi.fn(),
    setMemberRole: vi.fn(),
    leaveGroup: vi.fn(),
    uploadAttachment: vi.fn(),
    listContacts: vi.fn(),
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
  useAuth: () => ({ status: "authenticated", user: { id: h.ME } }),
}));

vi.mock("@/features/messaging/api", () => ({
  messagingApi: {
    listConversations: h.listConversations,
    listMessages: h.listMessages,
    sendMessage: h.sendMessage,
    markRead: h.markRead,
    openConversation: h.openConversation,
    getConversation: h.getConversation,
    createGroup: h.createGroup,
    updateGroup: h.updateGroup,
    listMembers: h.listMembers,
    addMembers: h.addMembers,
    removeMember: h.removeMember,
    setMemberRole: h.setMemberRole,
    leaveGroup: h.leaveGroup,
    uploadAttachment: h.uploadAttachment,
  },
}));

vi.mock("@/features/contacts/api", () => ({
  contactsApi: { list: h.listContacts },
}));

import { ConversationsView } from "@/features/messaging/ConversationsView";

function peer(id: string, name: string) {
  return {
    id,
    username: name.toLowerCase(),
    display_name: name,
    avatar_url: null,
    status_message: null,
    online: true,
    last_seen_at: null,
  };
}

function groupConversation(overrides: Record<string, unknown> = {}) {
  return {
    id: h.GROUP,
    kind: "group" as const,
    peer: null,
    title: "Team",
    avatar_url: null,
    member_count: 2,
    role: "owner" as const,
    last_message: null,
    unread_count: 0,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const DIRECT = {
  id: h.CONV,
  kind: "direct" as const,
  peer: peer(h.BOB, "Bob"),
  title: null,
  avatar_url: null,
  member_count: 2,
  role: "member" as const,
  last_message: null,
  unread_count: 0,
  created_at: "2026-01-01T00:00:00Z",
};

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

describe("ConversationsView groups & media", () => {
  beforeEach(() => {
    h.listeners.clear();
    [
      h.send,
      h.subscribe,
      h.listConversations,
      h.listMessages,
      h.sendMessage,
      h.markRead,
      h.openConversation,
      h.createGroup,
      h.updateGroup,
      h.listMembers,
      h.addMembers,
      h.removeMember,
      h.setMemberRole,
      h.leaveGroup,
      h.uploadAttachment,
      h.listContacts,
    ].forEach((fn) => fn.mockReset());

    Element.prototype.scrollIntoView = vi.fn();
    h.send.mockReturnValue(true);
    h.markRead.mockResolvedValue({ unread_count: 0 });
    h.listMessages.mockResolvedValue({ items: [], next_cursor: "" });
    h.listMembers.mockResolvedValue([]);
    h.listContacts.mockResolvedValue({
      items: [{ user: peer(h.CAROL, "Carol"), connected_at: "x" }],
      next_cursor: "",
    });
  });

  it("uploads a picked image and sends it as an image message", async () => {
    h.listConversations.mockResolvedValue({ items: [DIRECT], next_cursor: "" });
    h.uploadAttachment.mockResolvedValue({
      url: "/api/v1/media/attachments/abc.png",
      content_type: "image/png",
      kind: "image",
      name: "photo.png",
      size: 12,
    });
    h.sendMessage.mockResolvedValue({
      id: "m1",
      conversation_id: h.CONV,
      sender_id: h.ME,
      sequence_no: 1,
      client_msg_id: "c1",
      message_type: "image",
      content: { url: "/api/v1/media/attachments/abc.png" },
      status: "sent",
      created_at: "2026-01-01T00:00:00Z",
      edited_at: null,
      deleted_at: null,
    });

    const { container } = renderView();
    await userEvent.click(await screen.findByText("Bob"));

    const file = new File(["binary"], "photo.png", { type: "image/png" });
    const input = container.querySelector('input[type="file"]');
    expect(input).not.toBeNull();
    await userEvent.upload(input as HTMLInputElement, file);

    await waitFor(() => {
      expect(h.uploadAttachment).toHaveBeenCalledWith(
        file,
        expect.any(Function),
      );
    });
    await waitFor(() => {
      expect(h.sendMessage).toHaveBeenCalledWith(
        h.CONV,
        expect.objectContaining({ message_type: "image" }),
      );
    });
  });

  it("creates a group with the selected contacts", async () => {
    h.listConversations.mockResolvedValue({ items: [], next_cursor: "" });
    h.createGroup.mockResolvedValue(groupConversation({ member_count: 2 }));

    renderView();
    await userEvent.click(
      await screen.findByRole("button", { name: "New group" }),
    );

    await userEvent.type(screen.getByLabelText("Group name"), "Squad");
    await userEvent.click(await screen.findByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Create group" }));

    await waitFor(() => {
      expect(h.createGroup).toHaveBeenCalledWith({
        title: "Squad",
        member_ids: [h.CAROL],
      });
    });
  });

  it("manages group members and roles from the info dialog", async () => {
    h.listConversations.mockResolvedValue({
      items: [groupConversation()],
      next_cursor: "",
    });
    h.listMembers.mockResolvedValue([
      { ...peer(h.ME, "Me"), role: "owner", joined_at: "x" },
      { ...peer(h.BOB, "Bob"), role: "member", joined_at: "x" },
    ]);
    h.setMemberRole.mockResolvedValue(undefined);

    renderView();
    await userEvent.click(await screen.findByText("Team"));
    await userEvent.click(
      await screen.findByRole("button", { name: "Group info" }),
    );

    await screen.findByText("Bob");
    await userEvent.click(screen.getByRole("button", { name: "Make admin" }));

    await waitFor(() => {
      expect(h.setMemberRole).toHaveBeenCalledWith(h.GROUP, h.BOB, "admin");
    });
    expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
  });

  it("reflects a conversation.updated event in the header", async () => {
    h.listConversations.mockResolvedValue({
      items: [groupConversation()],
      next_cursor: "",
    });
    h.listMembers.mockResolvedValue([]);

    renderView();
    await userEvent.click(await screen.findByText("Team"));

    emit({
      type: "conversation.updated",
      conversation_id: h.GROUP,
      title: "Renamed",
      avatar_url: null,
      member_count: 3,
    });

    expect(
      (await screen.findAllByText("Renamed")).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("3 members")).toBeInTheDocument();
  });
});
