"use client";

import Link from "next/link";
import * as React from "react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { cn } from "@/lib/cn";
import { Avatar } from "@/features/messaging/Avatar";
import { relativeTime } from "@/features/messaging/format";
import { useNotifications } from "@/features/notifications/NotificationsProvider";
import type { Notification } from "@/features/notifications/types";

/** Human-readable sentence plus the destination for a notification. */
export function describeNotification(notification: Notification): {
  text: string;
  href: string;
} {
  const who = notification.actor?.display_name ?? "Someone";
  const preview =
    typeof notification.data?.preview === "string"
      ? notification.data.preview
      : "";

  switch (notification.type) {
    case "contact_request":
      return { text: `${who} sent you a friend request`, href: "/contacts" };
    case "contact_accepted":
      return { text: `${who} accepted your friend request`, href: "/contacts" };
    case "moment_like":
      return { text: `${who} liked your moment`, href: "/moments" };
    case "moment_comment":
      return {
        text: preview
          ? `${who} commented: ${preview}`
          : `${who} commented on your moment`,
        href: "/moments",
      };
    case "missed_call": {
      const kind = notification.data?.kind === "video" ? "video" : "voice";
      return { text: `Missed ${kind} call from ${who}`, href: "/messages" };
    }
    default:
      return { text: "New notification", href: "/notifications" };
  }
}

function NotificationRow({ notification }: { notification: Notification }) {
  const { markRead, remove } = useNotifications();
  const { text, href } = describeNotification(notification);
  const who = notification.actor?.display_name ?? "Notification";

  return (
    <li
      data-testid="notification-row"
      className={cn(
        "border-border-subtle flex items-start gap-3 border-b px-3 py-3 last:border-b-0",
        !notification.read && "bg-brand-50/60 dark:bg-brand-950/20",
      )}
    >
      <Avatar
        name={who}
        avatarUrl={notification.actor?.avatar_url ?? null}
        className="size-9"
      />
      <div className="min-w-0 flex-1">
        <Link
          href={href}
          onClick={() => {
            if (!notification.read) void markRead([notification.id]);
          }}
          className="text-ink hover:underline"
        >
          <span className="text-sm">{text}</span>
        </Link>
        <p className="text-ink-muted mt-0.5 text-xs">
          {relativeTime(notification.created_at)}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {!notification.read ? (
          <span
            aria-label="Unread"
            data-testid="notification-unread-dot"
            className="bg-brand-600 size-2 rounded-full"
          />
        ) : null}
        <Button
          variant="ghost"
          size="sm"
          aria-label={`Delete notification: ${text}`}
          onClick={() => void remove(notification.id)}
        >
          Delete
        </Button>
      </div>
    </li>
  );
}

/** The notifications list page body: history, mark-all-read, and clearing. */
export function NotificationsView() {
  const { items, unreadCount, loading, error, refresh, markRead, clear } =
    useNotifications();

  const hasUnread = unreadCount > 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-ink-muted text-sm" data-testid="notification-count">
          {items.length === 0
            ? "No notifications"
            : `${items.length} notification${items.length === 1 ? "" : "s"}`}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={!hasUnread}
            onClick={() => void markRead()}
          >
            Mark all read
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={items.length === 0}
            onClick={() => void clear()}
          >
            Clear all
          </Button>
        </div>
      </div>

      {error ? (
        <ErrorMessage message={error} onRetry={() => void refresh()} />
      ) : null}

      {loading && items.length === 0 ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState
          title="Nothing here yet"
          description="Friend requests, moment activity, and missed calls will appear here."
        />
      ) : (
        <ul className="border-border-subtle bg-surface-raised overflow-hidden rounded-lg border">
          {items.map((notification) => (
            <NotificationRow
              key={notification.id}
              notification={notification}
            />
          ))}
        </ul>
      )}
    </div>
  );
}
