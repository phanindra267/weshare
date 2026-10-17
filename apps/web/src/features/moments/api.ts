import { api } from "@/lib/api";
import type { Moment, MomentPage } from "./types";

export type ListFeedParams = {
  cursor?: string | null;
  limit?: number;
};

export type CreateMomentInput = {
  body?: string;
  visibility?: "friends" | "public" | "private";
  media?: { url: string; media_type: string }[];
};

export const momentsApi = {
  listFeed: (params: ListFeedParams = {}) => {
    const qs = new URLSearchParams();
    if (params.cursor) qs.set("cursor", params.cursor);
    if (params.limit) qs.set("limit", String(params.limit));
    const query = qs.toString() ? `?${qs.toString()}` : "";
    return api.get<MomentPage>(`/api/v1/moments${query}`);
  },
  create: (input: CreateMomentInput) =>
    api.post<Moment>("/api/v1/moments", { json: input }),
  get: (id: string) => api.get<Moment>(`/api/v1/moments/${id}`),
  delete: (id: string) => api.delete<void>(`/api/v1/moments/${id}`),
  like: (id: string) => api.post<{ like_count: number; liked: boolean }>(`/api/v1/moments/${id}/likes`),
  unlike: (id: string) => api.delete<{ like_count: number; liked: boolean }>(`/api/v1/moments/${id}/likes`),
};
