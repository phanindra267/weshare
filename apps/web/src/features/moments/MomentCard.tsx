"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { mediaUrl } from "@/lib/media";
import { initials, relativeTime } from "@/features/messaging/format";
import { momentsApi } from "./api";
import type { Comment, Moment } from "./types";

interface MomentCardProps {
  moment: Moment;
  currentUserId: string;
  onToggleLike: (moment: Moment) => void;
  onDelete: (moment: Moment) => void;
  onCommentCountChange: (momentId: string, delta: number) => void;
}

/** A single moment with its media, likes, and comments. */
export function MomentCard({
  moment,
  currentUserId,
  onToggleLike,
  onDelete,
  onCommentCountChange,
}: MomentCardProps) {
  const { toast } = useToast();
  const [commentsOpen, setCommentsOpen] = React.useState(false);
  const [comments, setComments] = React.useState<Comment[]>([]);
  const [nextCursor, setNextCursor] = React.useState<string | null>(null);
  const [loadingComments, setLoadingComments] = React.useState(false);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [draft, setDraft] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const isAuthor = moment.author.id === currentUserId;

  const loadComments = React.useCallback(
    async (cursor?: string | null) => {
      if (cursor) setLoadingMore(true);
      else setLoadingComments(true);
      try {
        const page = await momentsApi.listComments(moment.id, { cursor });
        setComments((prev) => (cursor ? [...prev, ...page.items] : page.items));
        setNextCursor(page.next_cursor ?? null);
      } catch (cause) {
        toast({
          title: "Could not load comments",
          description:
            cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      } finally {
        setLoadingComments(false);
        setLoadingMore(false);
      }
    },
    [moment.id, toast],
  );

  const handleToggleComments = () => {
    const next = !commentsOpen;
    setCommentsOpen(next);
    if (next && comments.length === 0) {
      void loadComments();
    }
  };

  const handleAddComment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const text = draft.trim();
    if (!text) return;
    setSubmitting(true);
    try {
      const comment = await momentsApi.addComment(moment.id, text);
      setComments((prev) => [...prev, comment]);
      setDraft("");
      onCommentCountChange(moment.id, 1);
    } catch (cause) {
      toast({
        title: "Could not add comment",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteComment = async (comment: Comment) => {
    try {
      await momentsApi.deleteComment(moment.id, comment.id);
      setComments((prev) => prev.filter((c) => c.id !== comment.id));
      onCommentCountChange(moment.id, -1);
    } catch (cause) {
      toast({
        title: "Could not delete comment",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    }
  };

  const handleDelete = async () => {
    if (!window.confirm("Delete this moment? This cannot be undone.")) return;
    setDeleting(true);
    try {
      await momentsApi.remove(moment.id);
      onDelete(moment);
    } catch (cause) {
      toast({
        title: "Could not delete moment",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setDeleting(false);
    }
  };

  return (
    <article className="border-border-subtle bg-surface-raised rounded-xl border p-4 shadow-sm">
      <header className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Avatar name={moment.author.display_name} url={moment.author.avatar_url} />
          <div>
            <p className="text-ink text-sm font-semibold">
              {moment.author.display_name}
            </p>
            <p className="text-ink-muted text-xs">
              {relativeTime(moment.created_at)}
              {isAuthor ? ` · ${moment.visibility}` : ""}
            </p>
          </div>
        </div>
        {isAuthor ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void handleDelete()}
            loading={deleting}
            aria-label="Delete moment"
          >
            Delete
          </Button>
        ) : null}
      </header>

      {moment.body ? (
        <p className="text-ink mt-3 text-sm leading-relaxed whitespace-pre-wrap">
          {moment.body}
        </p>
      ) : null}

      {moment.media.length > 0 ? (
        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {moment.media.map((media) => {
            const src = mediaUrl(media.url);
            if (!src) return null;
            return media.media_type === "video" ? (
              <video
                key={media.id}
                src={src}
                controls
                className="border-border-subtle w-full rounded-lg border"
              />
            ) : (
              <img
                key={media.id}
                src={src}
                alt="Moment attachment"
                className="border-border-subtle max-h-96 w-full rounded-lg border object-cover"
              />
            );
          })}
        </div>
      ) : null}

      <footer className="text-ink-muted mt-3 flex items-center gap-4 text-xs">
        <button
          type="button"
          onClick={() => onToggleLike(moment)}
          aria-pressed={moment.liked_by_me}
          aria-label={moment.liked_by_me ? "Unlike" : "Like"}
          className={
            moment.liked_by_me
              ? "text-brand-600 font-medium"
              : "hover:text-ink"
          }
        >
          {moment.liked_by_me ? "♥" : "♡"} {moment.like_count}
        </button>
        <button
          type="button"
          onClick={handleToggleComments}
          aria-expanded={commentsOpen}
          aria-label="Comments"
          className="hover:text-ink"
        >
          {moment.comment_count} comments
        </button>
      </footer>

      {commentsOpen ? (
        <div className="border-border-subtle mt-3 border-t pt-3">
          {loadingComments ? (
            <p className="text-ink-muted text-xs">Loading comments…</p>
          ) : comments.length === 0 ? (
            <p className="text-ink-muted text-xs">No comments yet.</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {comments.map((comment) => (
                <li key={comment.id} className="text-sm">
                  <span className="text-ink font-medium">
                    {comment.author.display_name}
                  </span>{" "}
                  <span className="text-ink">{comment.body}</span>{" "}
                  {comment.author.id === currentUserId ? (
                    <button
                      type="button"
                      onClick={() => void handleDeleteComment(comment)}
                      className="text-ink-muted hover:text-ink text-xs"
                      aria-label="Delete comment"
                    >
                      delete
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}

          {nextCursor ? (
            <button
              type="button"
              onClick={() => void loadComments(nextCursor)}
              disabled={loadingMore}
              className="text-ink-muted hover:text-ink mt-2 text-xs"
            >
              {loadingMore ? "Loading…" : "Load more comments"}
            </button>
          ) : null}

          <form onSubmit={handleAddComment} className="mt-3 flex gap-2">
            <input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="Write a comment…"
              aria-label="Write a comment"
              className="border-border-subtle bg-surface flex-1 rounded-lg border px-3 py-1.5 text-sm"
            />
            <Button type="submit" size="sm" loading={submitting} disabled={!draft.trim()}>
              Comment
            </Button>
          </form>
        </div>
      ) : null}
    </article>
  );
}

function Avatar({ name, url }: { name: string; url: string | null | undefined }) {
  const src = mediaUrl(url);
  return src ? (
    <img
      src={src}
      alt=""
      className="bg-surface-overlay size-9 shrink-0 rounded-full object-cover"
    />
  ) : (
    <span
      aria-hidden="true"
      className="bg-surface-overlay text-ink-muted grid size-9 shrink-0 place-items-center rounded-full text-xs font-medium"
    >
      {initials(name)}
    </span>
  );
}
