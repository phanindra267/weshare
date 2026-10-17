import { api } from "@/lib/api";
import type { Comment, Moment, MomentPage } from "./types";

export type ListFeedParams = {
  cursor?: string | null;
  limit?: number;
};

export type ListCommentsParams = {
  cursor?: string | null;
  limit?: number;
};

export type CreateMomentInput = {
  body?: string;
  visibility?: "friends" | "public" | "private";
  media?: { url: string; media_type: string }[];
};

/** Result of POST /api/v1/media/attachments. */
export interface UploadedAttachment {
  url: string;
  content_type: string;
  kind: "image" | "audio" | "file";
  name: string;
  size: number;
}

function pageQuery(params: { cursor?: string | null; limit?: number }): string {
  const qs = new URLSearchParams();
  if (params.cursor) qs.set("cursor", params.cursor);
  if (params.limit) qs.set("limit", String(params.limit));
  const query = qs.toString();
  return query ? `?${query}` : "";
}

export const momentsApi = {
  listFeed: (params: ListFeedParams = {}) =>
    api.get<MomentPage>(`/api/v1/moments${pageQuery(params)}`),

  create: (input: CreateMomentInput) =>
    api.post<Moment>("/api/v1/moments", { json: input }),

  get: (id: string) => api.get<Moment>(`/api/v1/moments/${id}`),

  remove: (id: string) => api.delete<void>(`/api/v1/moments/${id}`),

  like: (id: string) =>
    api.post<{ like_count: number; liked: boolean }>(
      `/api/v1/moments/${id}/likes`,
    ),

  unlike: (id: string) =>
    api.delete<{ like_count: number; liked: boolean }>(
      `/api/v1/moments/${id}/likes`,
    ),

  listComments: (id: string, params: ListCommentsParams = {}) =>
    api.get<{ items: Comment[]; next_cursor: string | null }>(
      `/api/v1/moments/${id}/comments${pageQuery(params)}`,
    ),

  addComment: (id: string, body: string) =>
    api.post<Comment>(`/api/v1/moments/${id}/comments`, { json: { body } }),

  deleteComment: (id: string, commentId: string) =>
    api.delete<void>(`/api/v1/moments/${id}/comments/${commentId}`),

  /** Upload a photo/video attachment for a moment. */
  uploadMedia: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return api.post<UploadedAttachment>("/api/v1/media/attachments", {
      formData: form,
    });
  },
};
