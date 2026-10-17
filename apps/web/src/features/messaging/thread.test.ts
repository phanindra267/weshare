import { describe, it, expect } from "vitest";
import { applyReceipt, mergeMany, mergeMessage } from "@/features/messaging/thread";
import type { Message, ThreadMessage } from "@/features/messaging/types";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    conversation_id: "c1",
    sender_id: "peer",
    sequence_no: 1,
    client_msg_id: null,
    message_type: "text",
    content: { text: "hi" },
    status: "received",
    created_at: "2026-01-01T00:00:00Z",
    edited_at: null,
    deleted_at: null,
    ...overrides,
  };
}

function pending(overrides: Partial<ThreadMessage> = {}): ThreadMessage {
  return {
    ...message(),
    id: "pending:abc",
    sender_id: "me",
    client_msg_id: "abc",
    sequence_no: Number.MAX_SAFE_INTEGER,
    status: "sent",
    pending: true,
    ...overrides,
  };
}

describe("mergeMessage", () => {
  it("inserts persisted messages in ascending sequence order", () => {
    const list: ThreadMessage[] = [
      { ...message({ id: "b", sequence_no: 2 }) },
    ];
    const merged = mergeMessage(list, message({ id: "a", sequence_no: 1 }));
    expect(merged.map((m) => m.id)).toEqual(["a", "b"]);
  });

  it("replaces an optimistic bubble when the server ack arrives", () => {
    const list: ThreadMessage[] = [pending()];
    const merged = mergeMessage(
      list,
      message({ id: "server-1", sender_id: "me", client_msg_id: "abc", sequence_no: 5, status: "sent" }),
    );
    expect(merged).toHaveLength(1);
    expect(merged[0].id).toBe("server-1");
    expect(merged[0].pending).toBeUndefined();
  });

  it("dedupes by server id", () => {
    const list: ThreadMessage[] = [{ ...message({ id: "server-1" }) }];
    const merged = mergeMessage(list, message({ id: "server-1", status: "read" }));
    expect(merged).toHaveLength(1);
    expect(merged[0].status).toBe("read");
  });
});

describe("mergeMany", () => {
  it("merges a batch preserving order and de-duplicating", () => {
    const list: ThreadMessage[] = [{ ...message({ id: "a", sequence_no: 1 }) }];
    const merged = mergeMany(list, [
      message({ id: "c", sequence_no: 3 }),
      message({ id: "b", sequence_no: 2 }),
      message({ id: "a", sequence_no: 1, status: "read" }),
    ]);
    expect(merged.map((m) => m.id)).toEqual(["a", "b", "c"]);
    expect(merged[0].status).toBe("read");
  });
});

describe("applyReceipt", () => {
  const outgoing = (id: string, seq: number, status: Message["status"]): ThreadMessage => ({
    ...message({ id, sender_id: "me", sequence_no: seq, status }),
  });

  it("advances the sender's messages up to the sequence", () => {
    const list = [outgoing("a", 1, "sent"), outgoing("b", 2, "sent"), outgoing("c", 3, "sent")];
    const result = applyReceipt(list, "me", 2, "delivered");
    expect(result.map((m) => m.status)).toEqual(["delivered", "delivered", "sent"]);
  });

  it("never downgrades a status and ignores other senders", () => {
    const list = [
      outgoing("a", 1, "read"),
      { ...message({ id: "peer", sender_id: "peer", sequence_no: 2, status: "received" }) },
    ];
    const result = applyReceipt(list, "me", 3, "delivered");
    expect(result[0].status).toBe("read");
    expect(result[1].status).toBe("received");
  });

  it("leaves pending messages untouched", () => {
    const list = [pending()];
    const result = applyReceipt(list, "me", 999, "read");
    expect(result[0].pending).toBe(true);
    expect(result[0].status).toBe("sent");
  });
});
