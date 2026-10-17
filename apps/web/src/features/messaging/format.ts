/** Presentation helpers for the messaging UI. */

/** Two-letter initials from a display name, falling back to "?". */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";
}

/** Short clock time for a message (e.g. "14:05"). */
export function shortTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Compact relative timestamp for a conversation list, e.g. "now", "5m", "3h",
 * "Tue", or a short date once it is older than a week.
 */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const diffMs = now.getTime() - date.getTime();
  const minutes = Math.floor(diffMs / 60_000);
  if (minutes < 1) return "now";
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d`;
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

/** Preview text for a message, derived from its type and content. */
export function messagePreview(message: {
  message_type: string;
  content: { text?: string; url?: string };
}): string {
  switch (message.message_type) {
    case "text":
    case "system":
      return message.content.text ?? "";
    case "image":
      return "Photo";
    case "file":
      return "Attachment";
    case "audio":
      return "Voice message";
    default:
      return "";
  }
}

/** Human-readable name for a conversation: group title or peer name. */
export function conversationName(conversation: {
  kind: string;
  title: string | null;
  peer: { display_name: string } | null;
}): string {
  if (conversation.kind === "group") {
    return conversation.title?.trim() || "Group";
  }
  return conversation.peer?.display_name ?? "Conversation";
}

/** Avatar for a conversation: group avatar or peer avatar. */
export function conversationAvatar(conversation: {
  kind: string;
  avatar_url: string | null;
  peer: { avatar_url: string | null } | null;
}): string | null {
  if (conversation.kind === "group") return conversation.avatar_url;
  return conversation.peer?.avatar_url ?? null;
}

/** Whether a conversation should show the peer's online indicator. */
export function conversationOnline(conversation: {
  kind: string;
  peer: { online: boolean } | null;
}): boolean {
  return conversation.kind !== "group" && Boolean(conversation.peer?.online);
}

/** Human-readable byte size, e.g. "1.2 MB". */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  const value = bytes / 1024 ** i;
  const rounded = i === 0 ? value : Math.round(value * 10) / 10;
  return `${rounded} ${units[i]}`;
}

/** Voice-note length, e.g. "0:07". */
export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds));
  const mins = Math.floor(total / 60);
  const secs = total % 60;
  return `${mins}:${String(secs).padStart(2, "0")}`;
}
