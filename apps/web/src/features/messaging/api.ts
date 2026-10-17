import { api } from "@/lib/api";
import { uploadWithProgress } from "@/lib/media";
import type { Paginated } from "@/types/api";
import type {
  Attachment,
  Conversation,
  CreateGroupInput,
  Member,
  Message,
  SendMessageInput,
  UpdateGroupInput,
} from "@/features/messaging/types";

interface ListConversationsParams {
  limit?: number;
  cursor?: string;
}

interface ListMessagesParams {
  limit?: number;
  /** Return messages older than this sequence (walking back through history). */
  before?: number;
  /** Return messages newer than this sequence (reconnect catch-up). */
  after?: number;
}

function conversationQuery(params: ListConversationsParams = {}): string {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

function messageQuery(params: ListMessagesParams = {}): string {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.before !== undefined) search.set("before", String(params.before));
  if (params.after !== undefined) search.set("after", String(params.after));
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

export const messagingApi = {
  listConversations: (params?: ListConversationsParams) =>
    api.get<Paginated<Conversation>>(
      `/api/v1/conversations${conversationQuery(params)}`,
    ),

  /** Find or create the direct conversation with a user. */
  openConversation: (userId: string) =>
    api.post<Conversation>("/api/v1/conversations", {
      json: { user_id: userId },
    }),

  getConversation: (id: string) =>
    api.get<Conversation>(`/api/v1/conversations/${id}`),

  listMessages: (id: string, params?: ListMessagesParams) =>
    api.get<Paginated<Message>>(
      `/api/v1/conversations/${id}/messages${messageQuery(params)}`,
    ),

  sendMessage: (id: string, input: SendMessageInput) =>
    api.post<Message>(`/api/v1/conversations/${id}/messages`, {
      json: input,
    }),

  markRead: (id: string, sequenceNo: number) =>
    api.post<{ unread_count: number }>(`/api/v1/conversations/${id}/read`, {
      json: { sequence_no: sequenceNo },
    }),

  // --- groups ---------------------------------------------------------------

  /** Create a group conversation owned by the caller. */
  createGroup: (input: CreateGroupInput) =>
    api.post<Conversation>("/api/v1/conversations/group", { json: input }),

  /** Update a group's title and/or avatar (owner or admin). */
  updateGroup: (id: string, input: UpdateGroupInput) =>
    api.patch<Conversation>(`/api/v1/conversations/${id}`, { json: input }),

  /** List a group's members. The endpoint wraps them in an `items` envelope. */
  listMembers: (id: string): Promise<Member[]> =>
    api
      .get<{ items: Member[] }>(`/api/v1/conversations/${id}/members`)
      .then((res) => res.items ?? []),

  /** Add contacts to a group; returns how many were newly added. */
  addMembers: (id: string, userIds: string[]) =>
    api.post<{ added: number }>(`/api/v1/conversations/${id}/members`, {
      json: { user_ids: userIds },
    }),

  removeMember: (id: string, userId: string) =>
    api.delete<void>(`/api/v1/conversations/${id}/members/${userId}`),

  setMemberRole: (id: string, userId: string, role: "admin" | "member") =>
    api.post<void>(`/api/v1/conversations/${id}/members/${userId}/role`, {
      json: { role },
    }),

  leaveGroup: (id: string) =>
    api.post<void>(`/api/v1/conversations/${id}/leave`),

  // --- media ----------------------------------------------------------------

  /** Upload a chat attachment (image, voice note, video, or generic file) with optional progress tracking. */
  uploadAttachment: (file: File, onProgress?: (percent: number) => void) => {
    if (onProgress) {
      return uploadWithProgress<Attachment>(file, onProgress);
    }
    const form = new FormData();
    form.append("file", file);
    return api.post<Attachment>("/api/v1/media/attachments", {
      formData: form,
    });
  },
};
