/**
 * Notifications contract types. Mirror the Go handlers in
 * `services/api/internal/notifications` and the realtime protocol in
 * `packages/contracts/websocket-events.md`.
 */

export type NotificationType =
  | "contact_request"
  | "contact_accepted"
  | "moment_like"
  | "moment_comment"
  | "missed_call";

/** Public profile of whoever triggered a notification. */
export interface NotificationActor {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
}

export interface Notification {
  id: string;
  type: NotificationType;
  actor: NotificationActor | null;
  conversation_id: string | null;
  message_id: string | null;
  moment_id: string | null;
  call_id: string | null;
  /** Small denormalised payload, e.g. `{ preview: "nice!" }`. */
  data: Record<string, unknown>;
  read: boolean;
  read_at: string | null;
  created_at: string;
}

/** Server-pushed frame when a notification is created for this user. */
export interface NotificationCreatedEvent {
  type: "notification.created";
  notification: Notification;
}
