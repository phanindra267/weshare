import { env } from "@/lib/env";
import type { ConnectionStatus, ServerEvent } from "@/features/messaging/types";

/** How often to send an application-level ping to keep the socket warm. */
const PING_INTERVAL_MS = 25_000;
/** Ceiling for the exponential reconnect backoff. */
const MAX_BACKOFF_MS = 15_000;
/** First reconnect delay; doubles each attempt. */
const BASE_BACKOFF_MS = 500;

type EventListener = (event: ServerEvent) => void;
type StatusListener = (status: ConnectionStatus) => void;

/**
 * A reconnecting WebSocket client for the realtime protocol.
 *
 * Authentication is by the HTTP-only session cookie, which the browser sends
 * on the handshake automatically. The client reconnects with exponential
 * backoff and notifies `onOpen` on every (re)connection so the app can resync
 * missed messages.
 */
export class RealtimeClient {
  private ws: WebSocket | null = null;
  private attempt = 0;
  private manuallyClosed = false;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private currentStatus: ConnectionStatus = "closed";
  private readonly eventListeners = new Set<EventListener>();
  private readonly statusListeners = new Set<StatusListener>();

  constructor(private readonly onOpen?: () => void) {}

  get status(): ConnectionStatus {
    return this.currentStatus;
  }

  /** Subscribe to server events. Returns an unsubscribe function. */
  onEvent(listener: EventListener): () => void {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  /** Subscribe to connection status. Fires immediately with the current value. */
  onStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener);
    listener(this.currentStatus);
    return () => this.statusListeners.delete(listener);
  }

  connect(): void {
    if (this.ws) return;
    this.manuallyClosed = false;
    this.open();
  }

  /** Send a client event. Returns false when the socket is not open. */
  send(event: unknown): boolean {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(event));
      return true;
    }
    return false;
  }

  /** Permanently close the client and stop reconnecting. */
  close(): void {
    this.manuallyClosed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.setStatus("closed");
    this.cleanupSocket();
    this.eventListeners.clear();
    this.statusListeners.clear();
  }

  private open(): void {
    this.setStatus("connecting");

    let socket: WebSocket;
    try {
      socket = new WebSocket(`${env.wsBaseUrl}/api/v1/realtime`);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws = socket;

    socket.onopen = () => {
      this.attempt = 0;
      this.setStatus("open");
      this.startPing();
      this.onOpen?.();
    };

    socket.onmessage = (event: MessageEvent) => {
      if (typeof event.data !== "string") return;
      let parsed: unknown;
      try {
        parsed = JSON.parse(event.data);
      } catch {
        return;
      }
      const serverEvent = parsed as ServerEvent;
      this.eventListeners.forEach((listener) => listener(serverEvent));
    };

    socket.onclose = () => {
      this.cleanupSocket();
      this.setStatus("closed");
      if (!this.manuallyClosed) this.scheduleReconnect();
    };

    socket.onerror = () => {
      // The close handler drives reconnection; just make sure the socket closes.
      try {
        socket.close();
      } catch {
        /* ignore */
      }
    };
  }

  private scheduleReconnect(): void {
    if (this.manuallyClosed || this.reconnectTimer) return;
    const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** this.attempt);
    this.attempt += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.open();
    }, delay);
  }

  private startPing(): void {
    this.stopPing();
    this.pingTimer = setInterval(() => {
      this.send({ type: "ping" });
    }, PING_INTERVAL_MS);
  }

  private stopPing(): void {
    if (this.pingTimer) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
  }

  private cleanupSocket(): void {
    this.stopPing();
    if (this.ws) {
      const socket = this.ws;
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      this.ws = null;
      try {
        socket.close();
      } catch {
        /* already closed */
      }
    }
  }

  private setStatus(status: ConnectionStatus): void {
    this.currentStatus = status;
    this.statusListeners.forEach((listener) => listener(status));
  }
}
