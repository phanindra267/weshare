"use client";

import * as React from "react";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { momentsApi } from "./api";
import type { Moment } from "./types";
import { MomentCard } from "./MomentCard";

interface MomentStackProps {
  moments: Moment[];
  setMoments: React.Dispatch<React.SetStateAction<Moment[]>>;
  currentUserId: string;
}

/**
 * Renders a list of moments with fully wired like, edit, delete, hide, and
 * comment interactions. Shared by the Moments feed, Discover, and search so
 * those interactions behave identically on every surface.
 */
export function MomentStack({
  moments,
  setMoments,
  currentUserId,
}: MomentStackProps) {
  const { toast } = useToast();

  const applyLike = React.useCallback(
    (id: string, liked: boolean, count?: number) => {
      setMoments((prev) =>
        prev.map((m) =>
          m.id === id
            ? {
                ...m,
                liked_by_me: liked,
                like_count: count ?? Math.max(0, m.like_count + (liked ? 1 : -1)),
              }
            : m,
        ),
      );
    },
    [setMoments],
  );

  const handleToggleLike = React.useCallback(
    async (moment: Moment) => {
      const liked = moment.liked_by_me;
      // Optimistic update.
      applyLike(moment.id, !liked);
      try {
        const result = liked
          ? await momentsApi.unlike(moment.id)
          : await momentsApi.like(moment.id);
        setMoments((prev) =>
          prev.map((m) =>
            m.id === moment.id
              ? {
                  ...m,
                  liked_by_me: result.liked,
                  like_count: result.like_count,
                }
              : m,
          ),
        );
      } catch (cause) {
        applyLike(moment.id, liked, moment.like_count);
        toast({
          title: "Could not update reaction",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      }
    },
    [applyLike, setMoments, toast],
  );

  const handleDelete = React.useCallback(
    (moment: Moment) => {
      setMoments((prev) => prev.filter((m) => m.id !== moment.id));
    },
    [setMoments],
  );

  const handleUpdate = React.useCallback(
    (updated: Moment) => {
      setMoments((prev) =>
        prev.map((m) => (m.id === updated.id ? updated : m)),
      );
    },
    [setMoments],
  );

  const handleHide = React.useCallback(
    (moment: Moment) => {
      setMoments((prev) => prev.filter((m) => m.id !== moment.id));
    },
    [setMoments],
  );

  const handleCommentCountChange = React.useCallback(
    (momentId: string, delta: number) => {
      setMoments((prev) =>
        prev.map((m) =>
          m.id === momentId
            ? { ...m, comment_count: Math.max(0, m.comment_count + delta) }
            : m,
        ),
      );
    },
    [setMoments],
  );

  return (
    <div className="flex flex-col gap-3" data-testid="moment-stack">
      {moments.map((moment) => (
        <MomentCard
          key={moment.id}
          moment={moment}
          currentUserId={currentUserId}
          onToggleLike={(m) => void handleToggleLike(m)}
          onDelete={handleDelete}
          onUpdate={handleUpdate}
          onHide={handleHide}
          onCommentCountChange={handleCommentCountChange}
        />
      ))}
    </div>
  );
}
