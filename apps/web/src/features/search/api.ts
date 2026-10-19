import { api } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type {
  ConversationSearchHit,
  MessageSearchHit,
  PostSearchHit,
  ProfileSearchHit,
} from "@/features/search/types";

export interface SearchParams {
  limit?: number;
  cursor?: string | null;
}

export interface UserSearchParams extends SearchParams {
  /** Restrict to groups (the `/search/groups` alias). */
  groupsOnly?: boolean;
}

function query(q: string, params: SearchParams = {}): string {
  const search = new URLSearchParams({ q });
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  return search.toString();
}

export const searchApi = {
  messages: (q: string, params?: SearchParams) =>
    api.get<Paginated<MessageSearchHit>>(
      `/api/v1/search/messages?${query(q, params)}`,
    ),

  users: (q: string, params?: SearchParams) =>
    api.get<Paginated<ProfileSearchHit>>(
      `/api/v1/search/users?${query(q, params)}`,
    ),

  conversations: (q: string, params?: UserSearchParams) => {
    const path = params?.groupsOnly
      ? "/api/v1/search/groups"
      : "/api/v1/search/conversations";
    return api.get<Paginated<ConversationSearchHit>>(
      `${path}?${query(q, params)}`,
    );
  },

  posts: (q: string, params?: SearchParams) =>
    api.get<Paginated<PostSearchHit>>(
      `/api/v1/search/posts?${query(q, params)}`,
    ),
};
