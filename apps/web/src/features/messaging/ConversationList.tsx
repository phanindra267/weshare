"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { mediaUrl } from "@/lib/media";
import {
  conversationAvatar,
  conversationName,
  conversationOnline,
  initials,
  messagePreview,
  relativeTime,
} from "@/features/messaging/format";
import type { Conversation } from "@/features/messaging/types";

export interface ConversationListProps {
  conversations: Conversation[];
  selectedId: string | null;
  typingConvIds: Set<string>;
  onSelect: (id: string) => void;
}

function Avatar({ name, avatarUrl }: { name: string; avatarUrl: string | null }) {
  const src = mediaUrl(avatarUrl);
  return src ? (
    <img
      src={src}
      alt=""
      className="bg-surface-overlay size-10 shrink-0 rounded-full object-cover"
    />
  ) : (
    <span
      aria-hidden="true"
      className="bg-surface-overlay text-ink-muted grid size-10 shrink-0 place-items-center rounded-full text-sm font-medium"
    >
      {initials(name)}
    </span>
  );
}

export function ConversationList({
  conversations,
  selectedId,
  typingConvIds,
  onSelect,
}: ConversationListProps) {
  return (
    <ul className="flex flex-col gap-1">
      {conversations.map((conversation) => {
        const active = conversation.id === selectedId;
        const typing = typingConvIds.has(conversation.id);
        const name = conversationName(conversation);
        const online = conversationOnline(conversation);
        const preview = conversation.last_message
          ? messagePreview(conversation.last_message)
          : "No messages yet";
        return (
          <li key={conversation.id}>
            <button
              type="button"
              onClick={() => onSelect(conversation.id)}
              aria-current={active ? "true" : undefined}
              className={cn(
                "flex w-full items-center gap-3 rounded-lg p-2 text-left transition-colors",
                active ? "bg-surface-overlay" : "hover:bg-surface-muted",
              )}
            >
              <span className="relative inline-flex shrink-0">
                <Avatar name={name} avatarUrl={conversationAvatar(conversation)} />
                {online ? (
                  <span
                    title="Online"
                    className="ring-surface-raised absolute right-0 bottom-0 size-3 rounded-full bg-green-500 ring-2"
                  />
                ) : null}
              </span>
              <span className="min-w-0 flex-1">
                <span className="flex items-center justify-between gap-2">
                  <span className="text-ink truncate text-sm font-medium">
                    {name}
                  </span>
                  {conversation.last_message ? (
                    <span className="text-ink-muted shrink-0 text-xs">
                      {relativeTime(conversation.last_message.created_at)}
                    </span>
                  ) : null}
                </span>
                <span className="flex items-center justify-between gap-2">
                  <span
                    className={cn(
                      "truncate text-xs",
                      typing ? "text-brand-600 italic" : "text-ink-muted",
                    )}
                  >
                    {typing ? "typing…" : preview}
                  </span>
                  {conversation.unread_count > 0 ? (
                    <span className="bg-brand-600 shrink-0 rounded-full px-1.5 text-[10px] font-semibold text-white">
                      {conversation.unread_count}
                    </span>
                  ) : null}
                </span>
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}
