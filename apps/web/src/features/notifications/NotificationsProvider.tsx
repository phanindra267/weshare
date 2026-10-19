"use client";

import * as React from "react";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { useRealtime } from "@/features/messaging/RealtimeProvider";
import { notificationsApi } from "@/features/notifications/api";
import type { Notification } from "@/features/notifications/types";

/** How many notifications the provider keeps in memory for the list page. */
const PAGE_SIZE = 30;

interface NotificationsContextValue {
  items: Notification[];
  unreadCount: number;
  loading: boolean;
  error: string | null;
  /** Reload the first page and the unread count. */
  refresh: () => Promise<void>;
  /** Mark the given notifications read; empty marks every unread one. */
  markRead: (ids?: string[]) => Promise<void>;
  /** Delete one notification. */
  remove: (id: string) => Promise<void>;
  /** Delete every notification for the current user. */
  clear: () => Promise<void>;
}

const NotificationsContext =
  React.createContext<NotificationsContextValue | null>(null);

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  return "Unable to load notifications.";
}

/**
 * Owns the notification list and unread badge for the authenticated session.
 * It reloads on every (re)connection so notifications created while the socket
 * was down are picked up, and prepends live `notification.created` frames.
 *
 * Mounted once, inside `RealtimeProvider` (it consumes the socket) and outside
 * the header (which renders the badge).
 */
export function NotificationsProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const { status, user } = useAuth();
  const { subscribe, connectionId } = useRealtime();

  const [items, setItems] = React.useState<Notification[]>([]);
  const [unreadCount, setUnreadCount] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  const authenticated = status === "authenticated";
  const userId = user?.id ?? null;

  // fetchData performs the network reads only; callers decide how to store the
  // result. Keeping setState out of it lets the effect below stay free of
  // synchronous state writes.
  const fetchData = React.useCallback(async () => {
    const [page, unread] = await Promise.all([
      notificationsApi.list({ limit: PAGE_SIZE }),
      notificationsApi.unreadCount(),
    ]);
    return { items: page.items, unreadCount: unread.unread_count };
  }, []);

  const refresh = React.useCallback(async () => {
    setLoading(true);
    try {
      const data = await fetchData();
      setItems(data.items);
      setUnreadCount(data.unreadCount);
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [fetchData]);

  // Load on sign-in, when the signed-in user changes, and on every reconnect
  // (catch-up for notifications created while the socket was down). State is
  // written only from the promise callbacks, never synchronously in the effect.
  React.useEffect(() => {
    if (!userId) return;
    let active = true;
    fetchData()
      .then((data) => {
        if (!active) return;
        setItems(data.items);
        setUnreadCount(data.unreadCount);
        setError(null);
      })
      .catch((err) => {
        if (active) setError(errorMessage(err));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [userId, connectionId, fetchData]);

  // Prepend live notifications.
  React.useEffect(() => {
    if (!userId) return;
    return subscribe((event) => {
      if (event.type !== "notification.created") return;
      const notification = event.notification;
      setItems((prev) =>
        prev.some((item) => item.id === notification.id)
          ? prev
          : [notification, ...prev].slice(0, PAGE_SIZE),
      );
      if (!notification.read) {
        setUnreadCount((count) => count + 1);
      }
    });
  }, [userId, subscribe]);

  const markRead = React.useCallback(async (ids?: string[]) => {
    const target = ids ?? [];
    // Optimistically flip the badge and the rows; reconcile from the server.
    setItems((prev) =>
      prev.map((item) =>
        target.length === 0 || target.includes(item.id)
          ? { ...item, read: true }
          : item,
      ),
    );
    if (target.length === 0) {
      setUnreadCount(0);
    } else {
      setUnreadCount((count) => Math.max(0, count - target.length));
    }
    try {
      const result = await notificationsApi.markRead(target);
      setUnreadCount(result.unread_count);
    } catch (err) {
      setError(errorMessage(err));
      void refresh();
    }
  }, [refresh]);

  const remove = React.useCallback(async (id: string) => {
    let wasUnread = false;
    setItems((prev) => {
      const found = prev.find((item) => item.id === id);
      wasUnread = found ? !found.read : false;
      return prev.filter((item) => item.id !== id);
    });
    if (wasUnread) setUnreadCount((count) => Math.max(0, count - 1));
    try {
      await notificationsApi.remove(id);
    } catch (err) {
      setError(errorMessage(err));
      void refresh();
    }
  }, [refresh]);

  const clear = React.useCallback(async () => {
    setItems([]);
    setUnreadCount(0);
    try {
      await notificationsApi.clear();
    } catch (err) {
      setError(errorMessage(err));
      void refresh();
    }
  }, [refresh]);

  // Signed-out visitors always see an empty, non-loading state so the badge
  // cannot leak the previous user's notifications.
  const value = React.useMemo(
    () =>
      authenticated
        ? {
            items,
            unreadCount,
            loading,
            error,
            refresh,
            markRead,
            remove,
            clear,
          }
        : {
            items: [],
            unreadCount: 0,
            loading: false,
            error: null,
            refresh,
            markRead,
            remove,
            clear,
          },
    [
      authenticated,
      items,
      unreadCount,
      loading,
      error,
      refresh,
      markRead,
      remove,
      clear,
    ],
  );

  return (
    <NotificationsContext.Provider value={value}>
      {children}
    </NotificationsContext.Provider>
  );
}

/** Access the notifications context; throws when used outside the provider. */
export function useNotifications(): NotificationsContextValue {
  const ctx = React.useContext(NotificationsContext);
  if (!ctx) {
    throw new Error(
      "useNotifications must be used within a NotificationsProvider",
    );
  }
  return ctx;
}
