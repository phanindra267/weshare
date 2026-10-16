"use client";

import * as React from "react";
import { useAuth } from "@/features/auth/AuthProvider";
import { presenceApi } from "@/features/presence/api";

/**
 * How often to report activity. Kept comfortably below the server's presence
 * staleness window so an active tab is never mistaken for offline.
 */
const HEARTBEAT_INTERVAL_MS = 30_000;

/**
 * Renders nothing; emits a presence heartbeat immediately and on an interval
 * while the visitor is authenticated. Failures are swallowed on purpose: a
 * missed heartbeat only affects presence display and must never surface as an
 * error or interrupt the UI.
 */
export function PresenceHeartbeat() {
  const { status } = useAuth();

  React.useEffect(() => {
    if (status !== "authenticated") return;

    let cancelled = false;
    const beat = () => {
      void presenceApi.sendHeartbeat().catch(() => {
        /* presence is best-effort */
      });
    };

    beat();
    const timer = setInterval(() => {
      if (!cancelled) beat();
    }, HEARTBEAT_INTERVAL_MS);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [status]);

  return null;
}
