import { api } from "@/lib/api";

export interface HeartbeatResult {
  online: boolean;
  last_seen_at: string;
}

export const presenceApi = {
  /**
   * Record activity. Presence is derived from `last_seen_at` by the API, so a
   * client that stops beating is naturally shown as offline.
   */
  sendHeartbeat: () =>
    api.post<HeartbeatResult>("/api/v1/presence/heartbeat"),
};
