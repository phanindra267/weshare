"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { mediaUrl } from "@/lib/media";
import {
  formatBytes,
  formatDuration,
  shortTime,
} from "@/features/messaging/format";
import type { ReplyPreview, ThreadMessage } from "@/features/messaging/types";

/** Quick reactions offered by the inline picker. */
const QUICK_REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🎉"];

export interface MessageListProps {
  messages: ThreadMessage[];
  currentUserId: string;
  /** Display name of a peer currently typing, if any. */
  typingName?: string | null;
  /** Display names keyed by user id, used to label senders in groups. */
  senderNames?: Record<string, string>;
  /** True when the viewer may pin messages (group admins/owners, or direct members). */
  canPin?: boolean;
  /** True when the viewer may delete another member's message (group admins). */
  canDeleteOthers?: boolean;
  /** A message to visually highlight (for example, jumped to from search). */
  highlightId?: string | null;
  onRetry: (message: ThreadMessage) => void;
  onReply?: (message: ThreadMessage) => void;
  onEdit?: (message: ThreadMessage) => void;
  onDelete?: (message: ThreadMessage) => void;
  onForward?: (message: ThreadMessage) => void;
  onReact?: (message: ThreadMessage, emoji: string) => void;
  onTogglePin?: (message: ThreadMessage, pinned: boolean) => void;
  /** Scroll to / highlight the quoted message of a reply. */
  onJumpTo?: (messageId: string) => void;
}

/** Delivery-status tick for the viewer's own outgoing messages. */
function StatusTicks({ status }: { status: ThreadMessage["status"] }) {
  const label =
    status === "read" ? "Read" : status === "delivered" ? "Delivered" : "Sent";
  const glyph = status === "sent" ? "✓" : "✓✓";
  return (
    <span
      title={label}
      aria-label={label}
      className={cn(
        "ml-1 text-[10px]",
        status === "read" ? "text-brand-600" : "text-ink-muted",
      )}
    >
      {glyph}
    </span>
  );
}

/** Short human label for a quoted message, used in reply previews. */
function replyPreviewLabel(preview: ReplyPreview): string {
  if (preview.deleted) return "Deleted message";
  switch (preview.message_type) {
    case "text":
      return preview.text || preview.content.text || "Message";
    case "image":
      return "Photo";
    case "video":
      return "Video";
    case "audio":
      return "Voice message";
    case "file":
      return preview.content.name ?? "Attachment";
    default:
      return "Message";
  }
}

function ImageBubble({ src }: { src: string }) {
  const [loaded, setLoaded] = React.useState(false);
  return (
    <a href={src} target="_blank" rel="noreferrer" className="block overflow-hidden rounded-lg">
      {!loaded && <div className="h-36 w-56 animate-pulse rounded-lg bg-surface-muted" />}
      <img
        src={src}
        alt="Shared image"
        className={cn("max-h-80 max-w-full rounded-lg object-contain", !loaded && "hidden")}
        onLoad={() => setLoaded(true)}
        onError={() => setLoaded(true)}
      />
    </a>
  );
}

/** Renders a message body for its type. Media urls are resolved to absolute. */
function MessageBody({ message }: { message: ThreadMessage }) {
  if (message.deleted_at) {
    return (
      <p className="text-xs italic opacity-80">This message was deleted.</p>
    );
  }
  const src = mediaUrl(message.content.url);
  switch (message.message_type) {
    case "image":
      return src ? <ImageBubble src={src} /> : <p>Photo</p>;
    case "video":
      return src ? (
        <div className="flex flex-col gap-1">
          <video
            controls
            preload="metadata"
            className="max-h-80 max-w-full rounded-lg"
            aria-label="Shared video"
          >
            <source src={src} type={message.content.content_type ?? "video/mp4"} />
            <a href={src} download className="text-xs underline">
              Download video
            </a>
          </video>
        </div>
      ) : (
        <p>Video</p>
      );
    case "audio":
      return src ? (
        <div className="flex flex-col gap-1">
          <audio controls preload="metadata" src={src} className="max-w-full" />
          {message.content.duration ? (
            <span className="text-[10px] opacity-80">
              {formatDuration(message.content.duration)}
            </span>
          ) : null}
        </div>
      ) : (
        <p>Voice message</p>
      );
    case "file":
      return src ? (
        <a
          href={src}
          download={message.content.name ?? undefined}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-2 underline"
        >
          <span aria-hidden="true">📎</span>
          <span>
            {message.content.name ?? "Attachment"}
            {message.content.size ? (
              <span className="ml-1 text-[10px] opacity-80">
                {formatBytes(message.content.size)}
              </span>
            ) : null}
          </span>
        </a>
      ) : (
        <p>Attachment</p>
      );
    default:
      return <p className="whitespace-pre-wrap break-words">{message.content.text}</p>;
  }
}

/** The quoted message a reply points at, rendered above the reply body. */
function ReplyQuote({
  preview,
  own,
  senderNames,
  onJumpTo,
}: {
  preview: ReplyPreview;
  own: boolean;
  senderNames?: Record<string, string>;
  onJumpTo?: (messageId: string) => void;
}) {
  const label = senderNames?.[preview.sender_id] ?? "Message";
  return (
    <button
      type="button"
      disabled={!onJumpTo}
      onClick={() => onJumpTo?.(preview.id)}
      className={cn(
        "mb-1 block w-full overflow-hidden rounded border-l-2 px-2 py-1 text-left text-[11px]",
        own
          ? "border-white/60 bg-white/10 text-white/90"
          : "border-brand-500 bg-surface-muted text-ink-muted",
        onJumpTo ? "cursor-pointer" : "cursor-default",
      )}
    >
      <span className={cn("block truncate font-medium", own ? "text-white" : "text-brand-600")}>
        {label}
      </span>
      <span className="block truncate">{replyPreviewLabel(preview)}</span>
    </button>
  );
}

/** Aggregate reaction chips plus an inline emoji picker. */
function Reactions({
  message,
  own,
  onReact,
}: {
  message: ThreadMessage;
  own: boolean;
  onReact?: (message: ThreadMessage, emoji: string) => void;
}) {
  const [pickerOpen, setPickerOpen] = React.useState(false);
  const reactions = message.reactions ?? [];
  if (!onReact && reactions.length === 0) return null;

  return (
    <div className={cn("mt-1 flex flex-wrap items-center gap-1", own ? "justify-end" : "justify-start")}>
      {reactions.map((reaction) => (
        <button
          key={reaction.emoji}
          type="button"
          aria-pressed={reaction.mine}
          aria-label={`${reaction.emoji} reaction, ${reaction.count}`}
          disabled={!onReact}
          onClick={() => onReact?.(message, reaction.emoji)}
          title={reaction.users.join(", ")}
          className={cn(
            "border-border-subtle bg-surface-overlay flex items-center gap-0.5 rounded-full border px-1.5 py-0.5 text-[11px]",
            reaction.mine && "border-brand-600 text-brand-600",
            onReact && "hover:border-brand-600",
          )}
        >
          <span aria-hidden="true">{reaction.emoji}</span>
          <span>{reaction.count}</span>
        </button>
      ))}
      {onReact ? (
        <div className="relative">
          <button
            type="button"
            aria-label="Add reaction"
            aria-expanded={pickerOpen}
            onClick={() => setPickerOpen((open) => !open)}
            className="border-border-subtle bg-surface-overlay text-ink-muted hover:text-ink flex h-6 items-center rounded-full border px-1.5 text-[11px]"
          >
            <span aria-hidden="true">＋</span>
          </button>
          {pickerOpen ? (
            <div className="border-border-subtle bg-surface-raised absolute bottom-full z-10 mb-1 flex gap-0.5 rounded-lg border p-1 shadow-lg">
              {QUICK_REACTIONS.map((emoji) => (
                <button
                  key={emoji}
                  type="button"
                  aria-label={`React with ${emoji}`}
                  onClick={() => {
                    onReact(message, emoji);
                    setPickerOpen(false);
                  }}
                  className="hover:bg-surface-muted rounded px-1 text-base leading-6"
                >
                  {emoji}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** A compact icon button used in the hover action bar. */
function ActionButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={onClick}
      className="border-border-subtle bg-surface-raised text-ink-muted hover:text-ink hover:border-brand-600 rounded-full border px-1.5 py-0.5 text-[11px] leading-4 shadow-sm"
    >
      {children}
    </button>
  );
}

function Bubble({
  message,
  own,
  senderName,
  senderNames,
  canPin,
  canDeleteOthers,
  highlighted,
  onRetry,
  onReply,
  onEdit,
  onDelete,
  onForward,
  onReact,
  onTogglePin,
  onJumpTo,
}: {
  message: ThreadMessage;
  own: boolean;
  senderName?: string;
  senderNames?: Record<string, string>;
  canPin: boolean;
  canDeleteOthers: boolean;
  highlighted: boolean;
  onRetry: (message: ThreadMessage) => void;
  onReply?: (message: ThreadMessage) => void;
  onEdit?: (message: ThreadMessage) => void;
  onDelete?: (message: ThreadMessage) => void;
  onForward?: (message: ThreadMessage) => void;
  onReact?: (message: ThreadMessage, emoji: string) => void;
  onTogglePin?: (message: ThreadMessage, pinned: boolean) => void;
  onJumpTo?: (messageId: string) => void;
}) {
  const time = shortTime(message.created_at);
  const canDelete = !message.deleted_at && (own || canDeleteOthers);

  return (
    <li
      id={`message-${message.id}`}
      className={cn(
        "group flex scroll-mt-4 rounded-lg",
        own ? "justify-end" : "justify-start",
        highlighted && "bg-brand-600/10",
      )}
    >
      <div className={cn("flex max-w-[75%] flex-col", own ? "items-end" : "items-start")}>
        {!own && senderName ? (
          <span className="text-ink-muted mb-0.5 px-1 text-[11px] font-medium">
            {senderName}
          </span>
        ) : null}
        <div className={cn("flex items-center gap-1", own ? "flex-row" : "flex-row-reverse")}>
          <div
            className={cn(
              "max-w-full rounded-2xl px-3 py-2 text-sm",
              own ? "bg-brand-600 text-white" : "bg-surface-overlay text-ink",
              message.failed && "ring-2 ring-red-500",
            )}
          >
            {message.reply_to ? (
              <ReplyQuote
                preview={message.reply_to}
                own={own}
                senderNames={senderNames}
                onJumpTo={onJumpTo}
              />
            ) : null}
            {message.forwarded_from ? (
              <p className={cn("mb-0.5 text-[10px] italic", own ? "text-white/80" : "text-ink-muted")}>
                Forwarded
              </p>
            ) : null}
            <MessageBody message={message} />
            <div
              className={cn(
                "mt-1 flex items-center gap-1 text-[10px]",
                own ? "text-white/70" : "text-ink-muted",
              )}
            >
              {message.pinned_at ? (
                <span title="Pinned" aria-label="Pinned">
                  📌
                </span>
              ) : null}
              <time dateTime={message.created_at}>{time}</time>
              {message.edited_at && !message.deleted_at ? <span>(edited)</span> : null}
              {own ? (
                message.pending ? (
                  <span aria-label="Sending">…</span>
                ) : message.failed ? (
                  <button type="button" onClick={() => onRetry(message)} className="underline">
                    Retry
                  </button>
                ) : (
                  <StatusTicks status={message.status} />
                )
              ) : null}
            </div>
          </div>

          {!message.pending && !message.deleted_at ? (
            <div className="flex shrink-0 flex-col gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
              {onReply ? (
                <ActionButton label="Reply" onClick={() => onReply(message)}>
                  ↩
                </ActionButton>
              ) : null}
              {onForward ? (
                <ActionButton label="Forward" onClick={() => onForward(message)}>
                  ↪
                </ActionButton>
              ) : null}
              {onEdit && own && message.message_type === "text" ? (
                <ActionButton label="Edit" onClick={() => onEdit(message)}>
                  ✎
                </ActionButton>
              ) : null}
              {onDelete && canDelete ? (
                <ActionButton label="Delete" onClick={() => onDelete(message)}>
                  🗑
                </ActionButton>
              ) : null}
              {onTogglePin && canPin ? (
                <ActionButton
                  label={message.pinned_at ? "Unpin" : "Pin"}
                  onClick={() => onTogglePin(message, !message.pinned_at)}
                >
                  📌
                </ActionButton>
              ) : null}
            </div>
          ) : null}
        </div>

        <Reactions message={message} own={own} onReact={message.deleted_at ? undefined : onReact} />
      </div>
    </li>
  );
}

/**
 * Scrollable message history with an auto-scroll anchor that follows new
 * messages. System messages render as centered notices; media messages render
 * inline. Sender names are labelled for group conversations. Replies, reactions,
 * pin badges, edit markers, and deleted tombstones are rendered for Phase 4.
 */
export function MessageList({
  messages,
  currentUserId,
  typingName,
  senderNames,
  canPin = false,
  canDeleteOthers = false,
  highlightId,
  onRetry,
  onReply,
  onEdit,
  onDelete,
  onForward,
  onReact,
  onTogglePin,
  onJumpTo,
}: MessageListProps) {
  const anchorRef = React.useRef<HTMLDivElement>(null);
  const lastId = messages.length > 0 ? messages[messages.length - 1].id : null;

  React.useEffect(() => {
    anchorRef.current?.scrollIntoView({ block: "end" });
  }, [lastId, typingName]);

  if (messages.length === 0) {
    return (
      <div className="text-ink-muted flex flex-1 items-center justify-center p-6 text-sm">
        No messages yet. Say hello.
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto p-4">
      <ul className="flex flex-col gap-1.5">
        {messages.map((message) =>
          message.message_type === "system" ? (
            <li key={message.id} className="flex justify-center">
              <p className="text-ink-muted bg-surface-muted rounded-full px-3 py-1 text-[11px]">
                {message.content.text}
              </p>
            </li>
          ) : (
            <Bubble
              key={message.id}
              message={message}
              own={message.sender_id === currentUserId}
              senderName={senderNames?.[message.sender_id]}
              senderNames={senderNames}
              canPin={canPin}
              canDeleteOthers={canDeleteOthers}
              highlighted={highlightId === message.id}
              onRetry={onRetry}
              onReply={onReply}
              onEdit={onEdit}
              onDelete={onDelete}
              onForward={onForward}
              onReact={onReact}
              onTogglePin={onTogglePin}
              onJumpTo={onJumpTo}
            />
          ),
        )}
      </ul>
      {typingName ? (
        <p className="text-ink-muted mt-2 px-1 text-xs" aria-live="polite">
          {typingName} is typing…
        </p>
      ) : null}
      <div ref={anchorRef} />
    </div>
  );
}
