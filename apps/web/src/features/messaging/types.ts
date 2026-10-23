/**
 * Messaging contract types. Mirror the Go handlers in
 * `services/api/internal/messaging` and the realtime protocol in
 * `packages/contracts/websocket-events.md`.
 */

import type { MomentCreatedEvent } from "@/features/moments/types";
import type { NotificationCreatedEvent } from "@/features/notifications/types";
import type {
  CallAcceptedEvent,
  CallBusyEvent,
  CallCandidateEvent,
  CallEndedEvent,
  CallIncomingEvent,
  CallParticipantsEvent,
  CallRejectedEvent,
  CallSDPEvent,
  CallUnavailableEvent,
} from "@/features/calls/types";

/** Public peer profile embedded in conversation payloads. */
export interface PeerBrief {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
}

export type MessageType = "text" | "image" | "file" | "audio" | "video" | "system";

/**
 * Delivery status as seen by the viewer. The sender's own messages move
 * sent → delivered → read; incoming messages are `received`.
 */
export type MessageStatus = "sent" | "delivered" | "read" | "received";

/** Message body; shape depends on `message_type`. */
export interface MessageContent {
  text?: string;
  url?: string;
  content_type?: string;
  /** Voice-note length in seconds (audio messages). */
  duration?: number;
  /** Original display name of an uploaded file. */
  name?: string;
  /** Size in bytes of an uploaded file. */
  size?: number;
}

/** One emoji's aggregate reactions on a message. */
export interface MessageReaction {
  emoji: string;
  count: number;
  /** True when the current viewer reacted with this emoji. */
  mine: boolean;
  users: string[];
}

/** Compact rendering of the message a reply quotes. */
export interface ReplyPreview {
  id: string;
  sender_id: string;
  message_type: MessageType;
  content: MessageContent;
  deleted: boolean;
  text: string;
}

export interface Message {
  id: string;
  conversation_id: string;
  sender_id: string;
  sequence_no: number;
  client_msg_id: string | null;
  message_type: MessageType;
  content: MessageContent;
  status: MessageStatus;
  created_at: string;
  edited_at: string | null;
  deleted_at: string | null;
  /** The quoted message when this is a reply; null otherwise. */
  reply_to: ReplyPreview | null;
  /** Id of the message this was forwarded from; null when not forwarded. */
  forwarded_from: string | null;
  /** When the message was pinned; null when not pinned. */
  pinned_at: string | null;
  /** Aggregated emoji reactions. */
  reactions: MessageReaction[];
}

/**
 * A message in the thread view. `pending` marks an optimistically-rendered
 * send that the server has not acknowledged yet; `failed` marks one whose send
 * errored and can be retried.
 */
export interface ThreadMessage extends Message {
  pending?: boolean;
  failed?: boolean;
}

export interface Conversation {
  id: string;
  kind: "direct" | "group";
  /** Populated for direct conversations; null for groups. */
  peer: PeerBrief | null;
  /** Group name; null for direct conversations. */
  title: string | null;
  /** Group avatar; null for direct conversations or when unset. */
  avatar_url: string | null;
  /** Number of members (groups); 2 for direct conversations. */
  member_count: number;
  /** The viewer's role in this conversation. */
  role: MemberRole;
  last_message: Message | null;
  unread_count: number;
  created_at: string;
}

/** A member's permission level within a group, ordered owner > admin > member. */
export type MemberRole = "owner" | "admin" | "member";

/** One group member joined with their public profile. */
export interface Member {
  id: string;
  role: MemberRole;
  display_name: string;
  username: string | null;
  avatar_url: string | null;
  status_message: string | null;
  online: boolean;
  last_seen_at: string | null;
  joined_at: string;
}

/** Result of uploading a file to the media store. */
export interface Attachment {
  id?: string;
  url: string;
  content_type: string;
  kind: "image" | "audio" | "video" | "file";
  name: string;
  size: number;
}

export interface OpenConversationInput {
  user_id: string;
}

export interface CreateGroupInput {
  title: string;
  member_ids: string[];
}

export interface UpdateGroupInput {
  title?: string;
  avatar_url?: string;
}

export interface SendMessageInput {
  client_msg_id: string;
  message_type: MessageType;
  content: MessageContent;
  /** Quote an earlier message in the same conversation (Phase 4). */
  reply_to_id?: string;
}

// --- realtime protocol ------------------------------------------------------

export interface HelloEvent {
  type: "hello";
  user_id: string;
  server_time: string;
}

export interface MessageCreatedEvent {
  type: "message.created";
  event_id: string;
  conversation_id: string;
  message: Message;
}

export interface MessageAckEvent {
  type: "message.ack";
  conversation_id: string;
  request_id?: string;
  message: Message;
}

export interface MessageReceiptEvent {
  type: "message.receipt";
  conversation_id: string;
  user_id: string;
  status: "delivered" | "read";
  up_to_sequence: number;
}

export interface TypingEvent {
  type: "typing";
  conversation_id: string;
  user_id: string;
  typing: boolean;
}

export interface PresenceEvent {
  type: "presence";
  user_id: string;
  online: boolean;
}

export interface SyncResultEvent {
  type: "sync.result";
  conversation_id: string;
  request_id?: string;
  messages: Message[];
  next_cursor: string;
}

export interface RealtimeErrorEvent {
  type: "error";
  code: string;
  message: string;
  request_id?: string;
}

export interface ReadAckEvent {
  type: "read.ack";
  conversation_id: string;
  request_id?: string;
  unread_count: number;
}

export interface PongEvent {
  type: "pong";
}

/** Group metadata changed (title or avatar). Broadcast to all members. */
export interface ConversationUpdatedEvent {
  type: "conversation.updated";
  conversation_id: string;
  title: string | null;
  avatar_url: string | null;
  member_count: number;
}

/** A group's membership changed. `action` explains the cause. */
export interface MemberChangedEvent {
  type: "member.changed";
  conversation_id: string;
  user_id: string;
  role: MemberRole | "";
  action: "added" | "removed" | "left" | "role";
}

/** A message's body was edited. */
export interface MessageUpdatedEvent {
  type: "message.updated";
  conversation_id: string;
  message: Message;
}

/** A message was tombstoned. `message` carries the redacted representation. */
export interface MessageDeletedEvent {
  type: "message.deleted";
  conversation_id: string;
  message_id: string;
  message: Message;
}

/** The full reaction set for one message changed. */
export interface MessageReactionEvent {
  type: "message.reaction";
  conversation_id: string;
  message_id: string;
  actor_id: string;
  reactions: MessageReaction[];
}

/** A message was pinned or unpinned. */
export interface MessagePinnedEvent {
  type: "message.pinned";
  conversation_id: string;
  message_id: string;
  actor_id: string;
  pinned: boolean;
}

export type ServerEvent =
  | HelloEvent
  | MessageCreatedEvent
  | MessageAckEvent
  | MessageReceiptEvent
  | TypingEvent
  | PresenceEvent
  | SyncResultEvent
  | ReadAckEvent
  | ConversationUpdatedEvent
  | MemberChangedEvent
  | MessageUpdatedEvent
  | MessageDeletedEvent
  | MessageReactionEvent
  | MessagePinnedEvent
  | MomentCreatedEvent
  | NotificationCreatedEvent
  | RealtimeErrorEvent
  | PongEvent
  | CallIncomingEvent
  | CallAcceptedEvent
  | CallRejectedEvent
  | CallBusyEvent
  | CallUnavailableEvent
  | CallSDPEvent
  | CallCandidateEvent
  | CallEndedEvent
  | CallParticipantsEvent;

export type ConnectionStatus = "connecting" | "open" | "closed";
