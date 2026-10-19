"use client";

import Link from "next/link";
import { useNotifications } from "@/features/notifications/NotificationsProvider";

/**
 * Header bell that links to the notifications page and shows an unread badge.
 * Renders nothing while signed out (the provider reports a zero count).
 */
export function NotificationBell() {
  const { unreadCount } = useNotifications();
  const label =
    unreadCount > 0
      ? `Notifications, ${unreadCount} unread`
      : "Notifications";

  return (
    <Link
      href="/notifications"
      aria-label={label}
      data-testid="notification-bell"
      className="text-ink-muted hover:text-ink relative inline-flex size-8 items-center justify-center rounded-md"
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.8}
        strokeLinecap="round"
        strokeLinejoin="round"
        className="size-5"
      >
        <path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
        <path d="M13.7 21a2 2 0 0 1-3.4 0" />
      </svg>
      {unreadCount > 0 ? (
        <span
          data-testid="notification-badge"
          className="bg-brand-600 absolute -top-0.5 -right-0.5 grid min-w-4 place-items-center rounded-full px-1 text-[10px] leading-4 font-semibold text-white"
        >
          {unreadCount > 99 ? "99+" : unreadCount}
        </span>
      ) : null}
    </Link>
  );
}
