import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { env } from "@/lib/env";
import { RealtimeClient } from "@/features/messaging/realtime";

class FakeWebSocket {
  static OPEN = 1;
  static CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readonly url: string;
  readyState = 0;
  closed = false;
  sent: string[] = [];
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.closed = true;
    this.readyState = FakeWebSocket.CLOSED;
  }

  emitOpen() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  emitMessage(payload: unknown) {
    this.onmessage?.({ data: JSON.stringify(payload) } as MessageEvent);
  }

  emitServerClose() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.(new CloseEvent("close"));
  }
}

describe("RealtimeClient", () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal("WebSocket", FakeWebSocket);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("connects to the realtime endpoint and reports status", () => {
    const statuses: string[] = [];
    const client = new RealtimeClient();
    client.onStatus((status) => statuses.push(status));

    expect(statuses).toEqual(["closed"]);
    client.connect();
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe(
      `${env.wsBaseUrl}/api/v1/realtime`,
    );
    expect(statuses).toEqual(["closed", "connecting"]);

    FakeWebSocket.instances[0].emitOpen();
    expect(statuses).toEqual(["closed", "connecting", "open"]);
    client.close();
  });

  it("dispatches parsed server events to subscribers", () => {
    const events: unknown[] = [];
    const client = new RealtimeClient();
    client.onEvent((event) => events.push(event));
    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.emitOpen();

    socket.emitMessage({ type: "hello", user_id: "u", server_time: "t" });

    expect(events).toEqual([
      { type: "hello", user_id: "u", server_time: "t" },
    ]);
    client.close();
  });

  it("only sends when the socket is open", () => {
    const client = new RealtimeClient();
    client.connect();
    const socket = FakeWebSocket.instances[0];

    expect(client.send({ type: "ping" })).toBe(false);
    socket.emitOpen();
    expect(client.send({ type: "ping" })).toBe(true);
    expect(socket.sent).toEqual([JSON.stringify({ type: "ping" })]);
    client.close();
  });

  it("reconnects with backoff after an unexpected close", () => {
    vi.useFakeTimers();
    const onOpen = vi.fn();
    const client = new RealtimeClient(onOpen);
    client.connect();
    FakeWebSocket.instances[0].emitOpen();
    expect(onOpen).toHaveBeenCalledTimes(1);

    FakeWebSocket.instances[0].emitServerClose();
    expect(client.status).toBe("closed");
    expect(FakeWebSocket.instances).toHaveLength(1);

    vi.advanceTimersByTime(600);
    expect(FakeWebSocket.instances).toHaveLength(2);
    FakeWebSocket.instances[1].emitOpen();
    expect(onOpen).toHaveBeenCalledTimes(2);
    client.close();
  });

  it("stops reconnecting after a manual close", () => {
    vi.useFakeTimers();
    const client = new RealtimeClient();
    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.emitOpen();

    client.close();
    expect(socket.closed).toBe(true);

    vi.advanceTimersByTime(60_000);
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
});
