"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { momentsApi } from "./api";
import type { Moment } from "./types";
import { MomentCard } from "./MomentCard";

export function MomentsView() {
  const { toast } = useToast();
  const [moments, setMoments] = React.useState<Moment[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [nextCursor, setNextCursor] = React.useState<string | null>(null);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [body, setBody] = React.useState("");
  const [posting, setPosting] = React.useState(false);

  const fetchFeed = React.useCallback(async (cursor?: string | null) => {
    if (!cursor) {
      setLoading(true);
    } else {
      setLoadingMore(true);
    }
    try {
      const page = await momentsApi.listFeed({ cursor });
      setMoments((prev) => (cursor ? [...prev, ...page.items] : page.items));
      setNextCursor(page.next_cursor ?? null);
      setError(null);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Unable to load moments.");
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, []);

  React.useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await fetchFeed();
      } catch {
        if (!cancelled) {
          setError("Unable to load moments.");
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [fetchFeed]);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = body.trim();
    if (!trimmed) return;
    setPosting(true);
    try {
      const moment = await momentsApi.create({ body: trimmed, visibility: "friends" });
      setMoments((prev) => [moment, ...prev]);
      setBody("");
    } catch (cause) {
      toast({
        title: "Could not share moment",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setPosting(false);
    }
  };

  if (loading && moments.length === 0) {
    return (
      <div className="mx-auto flex max-w-2xl flex-col gap-4">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (error && moments.length === 0) {
    return (
      <ErrorMessage
        title="Could not load moments"
        message={error}
        onRetry={() => void fetchFeed()}
      />
    );
  }

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-4">
      <form onSubmit={handleSubmit} className="border-border-subtle bg-surface-raised rounded-xl border p-4">
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="What's on your mind?"
          rows={3}
          className="border-border-subtle bg-surface w-full resize-none rounded-lg border px-3 py-2 text-sm"
        />
        <div className="mt-3 flex justify-end">
          <Button type="submit" loading={posting} disabled={!body.trim()}>
            Share
          </Button>
        </div>
      </form>

      {moments.length === 0 ? (
        <EmptyState
          title="No moments yet"
          description="When your friends share, they'll show up here."
        />
      ) : (
        <div className="flex flex-col gap-3">
          {moments.map((moment) => (
            <MomentCard key={moment.id} moment={moment} />
          ))}
        </div>
      )}

      {nextCursor ? (
        <div className="flex justify-center">
          <Button variant="ghost" size="sm" loading={loadingMore} onClick={() => void fetchFeed(nextCursor)}>
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  );
}
