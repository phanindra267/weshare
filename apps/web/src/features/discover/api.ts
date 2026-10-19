import { api } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type { ProfileSearchHit } from "@/features/search/types";
import type { Moment } from "@/features/moments/types";

export interface DiscoverParams {
  limit?: number;
  cursor?: string | null;
}

function query(params: DiscoverParams = {}): string {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

export const discoverApi = {
  /** People the caller is not yet connected to, newest accounts first. */
  people: (params?: DiscoverParams) =>
    api.get<Paginated<ProfileSearchHit>>(
      `/api/v1/discover/people${query(params)}`,
    ),

  /** Newest public moments from across the app. */
  moments: (params?: DiscoverParams) =>
    api.get<Paginated<Moment>>(`/api/v1/discover/moments${query(params)}`),
};
