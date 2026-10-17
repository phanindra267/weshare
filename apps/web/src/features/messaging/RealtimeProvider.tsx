"use client";

import * as React from "react";
import { useAuth } from "@/features/auth/AuthProvider";
import { RealtimeClient } from "@/features/messaging/realtime";
import type { ConnectionStatus, ServerEvent } from "@/features/messaging/types";

type Listener = (event: ServerEvent) => void;

interface RealtimeContextValue {
  status: ConnectionStatus;
  /**
   * Increments on every successful (re)connection. Consumers depend on it to
   * trigger a catch-up sync after the socket was down.
   */
  connectionId: number;
  /** Send a client event. Returns false when the socket is not open. */
  send: (event: unknown) => boolean;
  /** Subscribe to server events. Returns an unsubscribe function. */
  subscribe: (listener: Listener) => () => void;
}

const RealtimeContext = React.createContext<RealtimeContextValue | null>(null);

/**
 * Owns the single realtime WebSocket for the authenticated session and fans
 * server events out to subscribers. Mounted once, next to `AuthProvider`.
 *
 * The socket is opened only once the session resolves to authenticated and is
 * torn down on sign-out. It reconnects automatically; consumers watch
 * `connectionId` to resync anything they might have missed while offline.
 */
export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const { status: authStatus } = useAuth();
  const [status, setStatus] = React.useState<ConnectionStatus>("closed");
  const [connectionId, setConnectionId] = React.useState(0);
  const clientRef = React.useRef<RealtimeClient | null>(null);
  const listenersRef = React.useRef(new Set<Listener>());

  const handleOpen = React.useCallback(() => {
    setConnectionId((n) => n + 1);
  }, []);

  React.useEffect(() => {
    if (authStatus !== "authenticated") {
      return;
    }
    const client = new RealtimeClient(handleOpen);
    clientRef.current = client;

    const offStatus = client.onStatus(setStatus);
    const offEvent = client.onEvent((event) => {
      listenersRef.current.forEach((listener) => listener(event));
    });

    client.connect();

    return () => {
      offEvent();
      offStatus();
      client.close();
      clientRef.current = null;
    };
  }, [authStatus, handleOpen]);

  const subscribe = React.useCallback((listener: Listener) => {
    listenersRef.current.add(listener);
    return () => {
      listenersRef.current.delete(listener);
    };
  }, []);

  const send = React.useCallback(
    (event: unknown) => clientRef.current?.send(event) ?? false,
    [],
  );

  const value = React.useMemo<RealtimeContextValue>(
    () => ({ status, connectionId, send, subscribe }),
    [status, connectionId, send, subscribe],
  );

  return (
    <RealtimeContext.Provider value={value}>
      {children}
    </RealtimeContext.Provider>
  );
}

export function useRealtime(): RealtimeContextValue {
  const ctx = React.useContext(RealtimeContext);
  if (!ctx) {
    throw new Error("useRealtime must be used within <RealtimeProvider>.");
  }
  return ctx;
}
