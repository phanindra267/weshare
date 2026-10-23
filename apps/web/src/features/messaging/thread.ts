import type { Message, MessageStatus, ThreadMessage } from "@/features/messaging/types";

/** Higher rank wins when merging a status advance from a receipt. */
const STATUS_RANK: Record<MessageStatus, number> = {
  sent: 0,
  delivered: 1,
  received: 1,
  read: 2,
};

/** Pending (unsent) messages sort after every persisted one. */
function sortKey(message: ThreadMessage): number {
  return message.pending ? Number.MAX_SAFE_INTEGER : message.sequence_no;
}

function insertSorted(
  list: ThreadMessage[],
  message: ThreadMessage,
): ThreadMessage[] {
  const next = [...list];
  let i = next.length;
  while (i > 0 && sortKey(next[i - 1]) > sortKey(message)) i -= 1;
  next.splice(i, 0, message);
  return next;
}

/**
 * Insert or replace a message. Matching is by server id first, then by
 * `client_msg_id` so a server acknowledgement replaces the optimistic bubble.
 */
export function mergeMessage(
  list: ThreadMessage[],
  incoming: Message,
): ThreadMessage[] {
  const persisted: ThreadMessage = { ...incoming };

  const byId = list.findIndex((m) => m.id === incoming.id);
  if (byId >= 0) {
    const next = [...list];
    next[byId] = persisted;
    return next;
  }

  if (incoming.client_msg_id) {
    const byClient = list.findIndex(
      (m) => m.pending && m.client_msg_id === incoming.client_msg_id,
    );
    if (byClient >= 0) {
      const next = [...list];
      next[byClient] = persisted;
      return next;
    }
  }

  return insertSorted(list, persisted);
}

/** Merge a batch of messages, preserving ascending sequence order. */
export function mergeMany(
  list: ThreadMessage[],
  incoming: Message[],
): ThreadMessage[] {
  return incoming.reduce((acc, message) => mergeMessage(acc, message), list);
}

/** Advance the delivery status of the sender's messages up to a sequence. */
export function applyReceipt(
  list: ThreadMessage[],
  senderId: string,
  upTo: number,
  status: MessageStatus,
): ThreadMessage[] {
  const rank = STATUS_RANK[status];
  return list.map((message) => {
    if (message.sender_id !== senderId || message.pending) return message;
    if (message.sequence_no > upTo) return message;
    if (STATUS_RANK[message.status] >= rank) return message;
    return { ...message, status };
  });
}

/**
 * Replace a message's reaction set (from a `message.reaction` event or a
 * reaction REST response). No-op when the message is not in the thread.
 */
export function applyReactions(
  list: ThreadMessage[],
  messageId: string,
  reactions: ThreadMessage["reactions"],
): ThreadMessage[] {
  return list.map((message) =>
    message.id === messageId ? { ...message, reactions } : message,
  );
}

/**
 * Apply a pin/unpin. The event carries only a boolean, so the pinned timestamp
 * is approximated client-side; it is only used to show the pin badge.
 */
export function applyPinned(
  list: ThreadMessage[],
  messageId: string,
  pinned: boolean,
): ThreadMessage[] {
  return list.map((message) =>
    message.id === messageId
      ? { ...message, pinned_at: pinned ? new Date().toISOString() : null }
      : message,
  );
}
