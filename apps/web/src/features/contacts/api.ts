import { api } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type {
  Contact,
  ContactRequest,
  RequestDirection,
} from "@/features/contacts/types";

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

export const contactsApi = {
  list: (params?: ListParams) =>
    api.get<Paginated<Contact>>(`/api/v1/contacts${query(params)}`),

  listRequests: (direction: RequestDirection, params?: ListParams) => {
    const search = new URLSearchParams({ direction });
    if (params?.limit) search.set("limit", String(params.limit));
    if (params?.cursor) search.set("cursor", params.cursor);
    return api.get<Paginated<ContactRequest>>(
      `/api/v1/contacts/requests?${search.toString()}`,
    );
  },

  /** Send a friend request. Returns `accepted` immediately on a mutual add. */
  sendRequest: (userId: string) =>
    api.post<ContactRequest>("/api/v1/contacts/requests", {
      json: { user_id: userId },
    }),

  accept: (id: string) =>
    api.post<ContactRequest>(`/api/v1/contacts/requests/${id}/accept`),

  decline: (id: string) =>
    api.post<void>(`/api/v1/contacts/requests/${id}/decline`),

  remove: (userId: string) => api.delete<void>(`/api/v1/contacts/${userId}`),

  block: (userId: string) => api.post<void>(`/api/v1/contacts/${userId}/block`),

  unblock: (userId: string) =>
    api.delete<void>(`/api/v1/contacts/${userId}/block`),
};
