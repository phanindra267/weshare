"use client";

import * as React from "react";
import { Skeleton } from "@/components/ui/Skeleton";
import { ApiError } from "@/lib/api";
import { mediaUrl } from "@/lib/media";
import { contactsApi } from "@/features/contacts/api";
import type { UserBrief } from "@/features/contacts/types";

export interface MemberPickerProps {
  /** User ids that are already members and should not be offered. */
  excludeIds?: string[];
  /** Currently selected user ids. */
  selected: string[];
  /** Toggle a user id in/out of the selection. */
  onChange: (ids: string[]) => void;
  /** Optional id prefix so labels are unique when several pickers are mounted. */
  idPrefix?: string;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";
}

function Avatar({ user }: { user: UserBrief }) {
  const src = mediaUrl(user.avatar_url);
  return src ? (
    <img
      src={src}
      alt=""
      className="bg-surface-overlay size-8 shrink-0 rounded-full object-cover"
    />
  ) : (
    <span
      aria-hidden="true"
      className="bg-surface-overlay text-ink-muted grid size-8 shrink-0 place-items-center rounded-full text-xs font-medium"
    >
      {initials(user.display_name)}
    </span>
  );
}

/**
 * Checkbox list of the caller's accepted contacts, used to assemble a group's
 * initial membership or to invite people to an existing group. Only accepted
 * contacts are offered because the backend rejects non-contacts anyway.
 */
export function MemberPicker({
  excludeIds = [],
  selected,
  onChange,
  idPrefix = "member",
}: MemberPickerProps) {
  const [contacts, setContacts] = React.useState<UserBrief[] | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    contactsApi.list({ limit: 200 }).then(
      (page) => {
        if (cancelled) return;
        setContacts(page.items.map((c) => c.user));
      },
      (cause: unknown) => {
        if (cancelled) return;
        setError(
          cause instanceof ApiError
            ? cause.message
            : "Unable to load your contacts.",
        );
      },
    );
    return () => {
      cancelled = true;
    };
  }, []);

  if (error) {
    return (
      <p role="alert" className="text-xs text-red-600">
        {error}
      </p>
    );
  }

  if (!contacts) {
    return (
      <div className="space-y-2" aria-busy="true" aria-label="Loading contacts">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  const excluded = new Set(excludeIds);
  const selectable = contacts.filter((c) => !excluded.has(c.id));

  if (selectable.length === 0) {
    return (
      <p className="text-ink-muted text-sm">
        No contacts available to add. Connect with people first.
      </p>
    );
  }

  const toggle = (id: string) => {
    onChange(
      selected.includes(id)
        ? selected.filter((value) => value !== id)
        : [...selected, id],
    );
  };

  return (
    <ul className="border-border-subtle max-h-60 overflow-y-auto rounded-lg border">
      {selectable.map((user) => {
        const inputId = `${idPrefix}-${user.id}`;
        return (
          <li
            key={user.id}
            className="border-border-subtle flex items-center gap-3 border-b p-2 last:border-b-0"
          >
            <input
              id={inputId}
              type="checkbox"
              className="focus-visible:outline-brand-600 size-4 rounded focus-visible:outline-2"
              checked={selected.includes(user.id)}
              onChange={() => toggle(user.id)}
            />
            <label
              htmlFor={inputId}
              className="flex min-w-0 flex-1 cursor-pointer items-center gap-2"
            >
              <Avatar user={user} />
              <span className="min-w-0">
                <span className="text-ink block truncate text-sm">
                  {user.display_name}
                </span>
                {user.username ? (
                  <span className="text-ink-muted block truncate text-xs">
                    @{user.username}
                  </span>
                ) : null}
              </span>
            </label>
          </li>
        );
      })}
    </ul>
  );
}
