import { api } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type { Notification } from "@/features/notifications/types";

interface ListParams {
  limit?: number;
  cursor?: string;
}

function query(params: ListParams = {}): string {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

export const notificationsApi = {
  list: (params?: ListParams) =>
    api.get<Paginated<Notification>>(`/api/v1/notifications${query(params)}`),

  unreadCount: () =>
    api.get<{ unread_count: number }>("/api/v1/notifications/unread-count"),

  /** Mark the given notifications read, or every unread one when ids is empty. */
  markRead: (ids: string[] = []) =>
    api.post<{ unread_count: number }>("/api/v1/notifications/read", {
      json: { ids },
    }),

  remove: (id: string) => api.delete<void>(`/api/v1/notifications/${id}`),

  clear: () => api.delete<void>("/api/v1/notifications"),
};
