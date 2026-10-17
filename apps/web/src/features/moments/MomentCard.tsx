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
  onUpdate?: (moment: Moment) => void;
  onHide?: (moment: Moment) => void;
  onCommentCountChange: (momentId: string, delta: number) => void;
}

/** A single moment with its media, likes, comments, editing, hide and report actions. */
export function MomentCard({
  moment,
  currentUserId,
  onToggleLike,
  onDelete,
  onUpdate,
  onHide,
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

  // Edit post state
  const [isEditing, setIsEditing] = React.useState(false);
  const [editBody, setEditBody] = React.useState(moment.body);
  const [editVisibility, setEditVisibility] = React.useState(moment.visibility);
  const [savingEdit, setSavingEdit] = React.useState(false);

  // Hide & Report states
  const [hiding, setHiding] = React.useState(false);
  const [reportOpen, setReportOpen] = React.useState(false);
  const [reportReason, setReportReason] = React.useState("spam");
  const [reportDetails, setReportDetails] = React.useState("");
  const [submittingReport, setSubmittingReport] = React.useState(false);

  // Comment replies state
  const [replyingTo, setReplyingTo] = React.useState<Comment | null>(null);

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
      const comment = replyingTo
        ? await momentsApi.addComment(moment.id, text, replyingTo.id)
        : await momentsApi.addComment(moment.id, text);
      setComments((prev) => [...prev, comment]);
      setDraft("");
      setReplyingTo(null);
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

  const handleSaveEdit = async () => {
    setSavingEdit(true);
    try {
      const updated = await momentsApi.update(moment.id, {
        body: editBody.trim(),
        visibility: editVisibility,
      });
      setIsEditing(false);
      onUpdate?.(updated);
      toast({
        title: "Post updated",
        variant: "success",
      });
    } catch (cause) {
      toast({
        title: "Could not update post",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setSavingEdit(false);
    }
  };

  const handleHide = async () => {
    setHiding(true);
    try {
      await momentsApi.hide(moment.id);
      onHide?.(moment);
      toast({
        title: "Post hidden",
        description: "This post will no longer appear in your feed.",
        variant: "success",
      });
    } catch (cause) {
      toast({
        title: "Could not hide post",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setHiding(false);
    }
  };

  const handleReport = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmittingReport(true);
    try {
      await momentsApi.report(moment.id, {
        reason: reportReason,
        details: reportDetails.trim() || undefined,
      });
      setReportOpen(false);
      setReportDetails("");
      toast({
        title: "Report submitted",
        description: "Thank you for helping keep the community safe.",
        variant: "success",
      });
    } catch (cause) {
      toast({
        title: "Could not submit report",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setSubmittingReport(false);
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
        <div className="flex items-center gap-1">
          {isAuthor ? (
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setEditBody(moment.body);
                  setEditVisibility(moment.visibility);
                  setIsEditing((prev) => !prev);
                }}
                aria-label="Edit moment"
              >
                {isEditing ? "Cancel" : "Edit"}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void handleDelete()}
                loading={deleting}
                aria-label="Delete moment"
              >
                Delete
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void handleHide()}
                loading={hiding}
                aria-label="Hide moment"
              >
                Hide
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setReportOpen((prev) => !prev)}
                aria-label="Report moment"
              >
                Report
              </Button>
            </>
          )}
        </div>
      </header>

      {/* Report Post Form */}
      {reportOpen ? (
        <form
          onSubmit={handleReport}
          className="border-border-subtle bg-surface mt-3 rounded-lg border p-3 text-sm"
        >
          <p className="text-ink font-semibold text-xs mb-2">Report Post</p>
          <div className="flex flex-col gap-2">
            <select
              value={reportReason}
              onChange={(e) => setReportReason(e.target.value)}
              aria-label="Report Reason"
              className="border-border-subtle bg-surface text-ink rounded-md border px-2 py-1 text-xs"
            >
              <option value="spam">Spam / Advertising</option>
              <option value="harassment">Harassment or Hate Speech</option>
              <option value="inappropriate">Inappropriate Content</option>
              <option value="misinformation">Misinformation</option>
              <option value="other">Other</option>
            </select>
            <textarea
              value={reportDetails}
              onChange={(e) => setReportDetails(e.target.value)}
              placeholder="Optional additional details..."
              rows={2}
              className="border-border-subtle bg-surface text-ink resize-none rounded-md border px-2 py-1 text-xs"
            />
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setReportOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" size="sm" loading={submittingReport}>
                Submit Report
              </Button>
            </div>
          </div>
        </form>
      ) : null}

      {/* Body / Inline Edit Mode */}
      {isEditing ? (
        <div className="mt-3 flex flex-col gap-2">
          <textarea
            value={editBody}
            onChange={(e) => setEditBody(e.target.value)}
            rows={3}
            aria-label="Edit post body"
            className="border-border-subtle bg-surface text-ink w-full resize-none rounded-lg border px-3 py-2 text-sm"
          />
          <div className="flex items-center justify-between gap-2">
            <select
              value={editVisibility}
              onChange={(e) =>
                setEditVisibility(e.target.value as "friends" | "public" | "private")
              }
              aria-label="Edit visibility"
              className="border-border-subtle bg-surface text-ink rounded-md border px-2 py-1 text-xs"
            >
              <option value="friends">Friends</option>
              <option value="public">Public</option>
              <option value="private">Only me</option>
            </select>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setIsEditing(false)}
              >
                Cancel
              </Button>
              <Button
                type="button"
                size="sm"
                loading={savingEdit}
                onClick={() => void handleSaveEdit()}
              >
                Save
              </Button>
            </div>
          </div>
        </div>
      ) : moment.body ? (
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
                  {comment.reply_to_author ? (
                    <div className="text-ink-muted text-xs mb-0.5">
                      ↳ replying to @{comment.reply_to_author.display_name}
                    </div>
                  ) : null}
                  <span className="text-ink font-medium">
                    {comment.author.display_name}
                  </span>{" "}
                  <span className="text-ink">{comment.body}</span>{" "}
                  <div className="inline-flex gap-2 text-xs text-ink-muted">
                    <button
                      type="button"
                      onClick={() => {
                        setReplyingTo(comment);
                      }}
                      className="hover:text-ink"
                      aria-label={`Reply to ${comment.author.display_name}`}
                    >
                      reply
                    </button>
                    {comment.author.id === currentUserId || isAuthor ? (
                      <button
                        type="button"
                        onClick={() => void handleDeleteComment(comment)}
                        className="hover:text-ink"
                        aria-label="Delete comment"
                      >
                        delete
                      </button>
                    ) : null}
                  </div>
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

          {/* Reply indicator banner */}
          {replyingTo ? (
            <div className="bg-surface border-border-subtle mt-2 flex items-center justify-between rounded-lg border px-2.5 py-1 text-xs">
              <span className="text-ink-muted">
                Replying to{" "}
                <strong className="text-ink font-medium">
                  @{replyingTo.author.display_name}
                </strong>
              </span>
              <button
                type="button"
                onClick={() => setReplyingTo(null)}
                className="text-ink-muted hover:text-ink font-bold"
                aria-label="Cancel reply"
              >
                ×
              </button>
            </div>
          ) : null}

          <form onSubmit={handleAddComment} className="mt-3 flex gap-2">
            <input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={
                replyingTo
                  ? `Reply to @${replyingTo.author.display_name}…`
                  : "Write a comment…"
              }
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
