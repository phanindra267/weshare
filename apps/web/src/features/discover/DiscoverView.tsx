"use client";

import Link from "next/link";
import * as React from "react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { Avatar } from "@/features/messaging/Avatar";
import { contactsApi } from "@/features/contacts/api";
import { MomentStack } from "@/features/moments/MomentStack";
import type { Moment } from "@/features/moments/types";
import type { ProfileSearchHit } from "@/features/search/types";
import { discoverApi } from "./api";

const PEOPLE_LIMIT = 6;
const MOMENTS_LIMIT = 10;

/**
 * Discover landing page: people the caller may know, public moments from across
 * the app, and a shortcut into global search.
 */
export function DiscoverView() {
  const { user } = useAuth();
  const { toast } = useToast();
  const currentUserId = user?.id ?? "";

  const [people, setPeople] = React.useState<ProfileSearchHit[]>([]);
  const [peopleLoading, setPeopleLoading] = React.useState(true);
  const [peopleError, setPeopleError] = React.useState<string | null>(null);
  const [peopleReload, setPeopleReload] = React.useState(0);
  const [addedIds, setAddedIds] = React.useState<Set<string>>(new Set());

  const [moments, setMoments] = React.useState<Moment[]>([]);
  const [momentsLoading, setMomentsLoading] = React.useState(true);
  const [momentsError, setMomentsError] = React.useState<string | null>(null);
  const [momentsCursor, setMomentsCursor] = React.useState<string | null>(null);
  const [momentsReload, setMomentsReload] = React.useState(0);
  const [loadingMore, setLoadingMore] = React.useState(false);

  // The first page of each section is fetched in an effect that writes state
  // only from promise callbacks (never synchronously in the effect body). A
  // reload token re-runs the effect for retries.
  React.useEffect(() => {
    let active = true;
    discoverApi
      .people({ limit: PEOPLE_LIMIT })
      .then((page) => {
        if (!active) return;
        setPeople(page.items);
        setPeopleError(null);
      })
      .catch((cause) => {
        if (!active) return;
        setPeopleError(
          cause instanceof ApiError ? cause.message : "Could not load people.",
        );
      })
      .finally(() => {
        if (active) setPeopleLoading(false);
      });
    return () => {
      active = false;
    };
  }, [peopleReload]);

  React.useEffect(() => {
    let active = true;
    discoverApi
      .moments({ limit: MOMENTS_LIMIT })
      .then((page) => {
        if (!active) return;
        setMoments(page.items);
        setMomentsCursor(page.next_cursor ?? null);
        setMomentsError(null);
      })
      .catch((cause) => {
        if (!active) return;
        setMomentsError(
          cause instanceof ApiError ? cause.message : "Could not load moments.",
        );
      })
      .finally(() => {
        if (active) setMomentsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [momentsReload]);

  function retryPeople() {
    setPeopleLoading(true);
    setPeopleReload((n) => n + 1);
  }

  function retryMoments() {
    setMomentsLoading(true);
    setMomentsReload((n) => n + 1);
  }

  async function loadMoreMoments() {
    if (!momentsCursor) return;
    setLoadingMore(true);
    try {
      const page = await discoverApi.moments({
        cursor: momentsCursor,
        limit: MOMENTS_LIMIT,
      });
      setMoments((prev) => [...prev, ...page.items]);
      setMomentsCursor(page.next_cursor ?? null);
    } catch (cause) {
      toast({
        title: "Could not load more moments",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setLoadingMore(false);
    }
  }

  async function addFriend(profile: ProfileSearchHit) {
    try {
      const result = await contactsApi.sendRequest(profile.id);
      setAddedIds((prev) => new Set(prev).add(profile.id));
      toast({
        title:
          result.status === "accepted"
            ? `You are now friends with ${profile.display_name}`
            : `Friend request sent to ${profile.display_name}`,
        variant: "success",
      });
    } catch (err) {
      toast({
        title: "Could not send request",
        description: err instanceof ApiError ? err.message : undefined,
        variant: "error",
      });
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link
        href="/search"
        className="border-border-subtle bg-surface-raised text-ink-muted hover:border-brand-500 hover:text-ink flex items-center gap-3 rounded-lg border px-4 py-3 text-sm transition-colors"
      >
        <span aria-hidden="true">🔍</span>
        <span>Search messages, people, chats, and posts</span>
      </Link>

      <section className="flex flex-col gap-3" aria-labelledby="discover-people">
        <div className="flex items-center justify-between">
          <h2 id="discover-people" className="text-lg font-semibold">
            People you may know
          </h2>
          <Link href="/contacts" className="text-brand-600 text-sm hover:underline">
            Contacts
          </Link>
        </div>

        {peopleLoading ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : peopleError ? (
          <ErrorMessage message={peopleError} onRetry={retryPeople} />
        ) : people.length === 0 ? (
          <EmptyState
            title="No suggestions right now"
            description="You are already connected to everyone we could find."
          />
        ) : (
          <ul
            data-testid="discover-people-results"
            className="border-border-subtle bg-surface-raised overflow-hidden rounded-lg border"
          >
            {people.map((profile) => {
              const added = addedIds.has(profile.id);
              return (
                <li
                  key={profile.id}
                  className="border-border-subtle flex items-center gap-3 border-b px-3 py-3 last:border-b-0"
                >
                  <Avatar
                    name={profile.display_name}
                    avatarUrl={profile.avatar_url}
                    className="size-9"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="text-ink truncate text-sm font-medium">
                      {profile.display_name}
                    </p>
                    <p className="text-ink-muted truncate text-xs">
                      {profile.username ? `@${profile.username}` : ""}
                      {profile.status_message
                        ? ` · ${profile.status_message}`
                        : ""}
                    </p>
                  </div>
                  {added ? (
                    <Link href="/messages">
                      <Button variant="secondary" size="sm">
                        Message
                      </Button>
                    </Link>
                  ) : (
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => void addFriend(profile)}
                    >
                      Add friend
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>

      <section className="flex flex-col gap-3" aria-labelledby="discover-moments">
        <h2 id="discover-moments" className="text-lg font-semibold">
          Discover moments
        </h2>

        {momentsLoading ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        ) : momentsError ? (
          <ErrorMessage message={momentsError} onRetry={retryMoments} />
        ) : moments.length === 0 ? (
          <EmptyState
            title="No public moments yet"
            description="Public moments shared by people across WeShare will appear here."
          />
        ) : (
          <>
            <MomentStack
              moments={moments}
              setMoments={setMoments}
              currentUserId={currentUserId}
            />
            {momentsCursor ? (
              <div className="flex justify-center">
                <Button
                  variant="ghost"
                  size="sm"
                  loading={loadingMore}
                  onClick={() => void loadMoreMoments()}
                >
                  Load more
                </Button>
              </div>
            ) : null}
          </>
        )}
      </section>
    </div>
  );
}
