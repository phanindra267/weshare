"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { mediaUrl } from "@/lib/media";
import {
  formatBytes,
  formatDuration,
  shortTime,
} from "@/features/messaging/format";
import type { ThreadMessage } from "@/features/messaging/types";

export interface MessageListProps {
  messages: ThreadMessage[];
  currentUserId: string;
  /** Display name of a peer currently typing, if any. */
  typingName?: string | null;
  /** Display names keyed by user id, used to label senders in groups. */
  senderNames?: Record<string, string>;
  onRetry: (message: ThreadMessage) => void;
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

/** Renders a message body for its type. Media urls are resolved to absolute. */
function MessageBody({ message }: { message: ThreadMessage }) {
  const src = mediaUrl(message.content.url);
  switch (message.message_type) {
    case "image":
      return src ? (
        <a href={src} target="_blank" rel="noreferrer">
          <img src={src} alt="Shared image" className="max-w-full rounded-lg" />
        </a>
      ) : (
        <p>Photo</p>
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
          <span className="flex flex-col">
            <span className="font-medium">
              {message.content.name ?? "Attachment"}
            </span>
            {message.content.size ? (
              <span className="text-[10px] opacity-80">
                {formatBytes(message.content.size)}
              </span>
            ) : null}
          </span>
        </a>
      ) : (
        <p>Attachment</p>
      );
    default:
      return (
        <p className="whitespace-pre-wrap break-words">{message.content.text}</p>
      );
  }
}

function Bubble({
  message,
  own,
  senderName,
  onRetry,
}: {
  message: ThreadMessage;
  own: boolean;
  senderName?: string;
  onRetry: (message: ThreadMessage) => void;
}) {
  const time = shortTime(message.created_at);
  return (
    <li className={cn("flex", own ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "flex max-w-[75%] flex-col",
          own ? "items-end" : "items-start",
        )}
      >
        {!own && senderName ? (
          <span className="text-ink-muted mb-0.5 px-1 text-[11px] font-medium">
            {senderName}
          </span>
        ) : null}
        <div
          className={cn(
            "max-w-full rounded-2xl px-3 py-2 text-sm",
            own ? "bg-brand-600 text-white" : "bg-surface-overlay text-ink",
            message.failed && "ring-2 ring-red-500",
          )}
        >
          <MessageBody message={message} />
          <div
            className={cn(
              "mt-1 flex items-center gap-1 text-[10px]",
              own ? "text-white/70" : "text-ink-muted",
            )}
          >
            <time dateTime={message.created_at}>{time}</time>
            {own ? (
              message.pending ? (
                <span aria-label="Sending">…</span>
              ) : message.failed ? (
                <button
                  type="button"
                  onClick={() => onRetry(message)}
                  className="underline"
                >
                  Retry
                </button>
              ) : (
                <StatusTicks status={message.status} />
              )
            ) : null}
          </div>
        </div>
      </div>
    </li>
  );
}

/**
 * Scrollable message history with an auto-scroll anchor that follows new
 * messages. System messages render as centered notices; media messages render
 * inline. Sender names are labelled for group conversations.
 */
export function MessageList({
  messages,
  currentUserId,
  typingName,
  senderNames,
  onRetry,
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
              onRetry={onRetry}
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
