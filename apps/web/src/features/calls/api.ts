import { api } from "@/lib/api";
import type { Paginated } from "@/types/api";
import type { CallHistoryItem, CallIceConfig } from "@/features/calls/types";

export const callsApi = {
  getIceConfig: () => api.get<CallIceConfig>("/api/v1/calls/ice"),
  getHistory: (limit?: number) => {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    return api.get<Paginated<CallHistoryItem>>(
      `/api/v1/calls${qs ? `?${qs}` : ""}`,
    );
  },
};
