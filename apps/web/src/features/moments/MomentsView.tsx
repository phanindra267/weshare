"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { useRealtime } from "@/features/messaging/RealtimeProvider";
import { contactsApi } from "@/features/contacts/api";
import type { Contact } from "@/features/contacts/types";
import { momentsApi, type UploadedAttachment } from "./api";
import type { Moment } from "./types";
import { MomentStack } from "./MomentStack";

type Visibility = "friends" | "public" | "private";

const MAX_MEDIA = 9;

export function MomentsView() {
  const { user } = useAuth();
  const { toast } = useToast();
  const { subscribe } = useRealtime();
  const currentUserId = user?.id ?? "";

  const [moments, setMoments] = React.useState<Moment[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [nextCursor, setNextCursor] = React.useState<string | null>(null);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [body, setBody] = React.useState("");
  const [visibility, setVisibility] = React.useState<Visibility>("friends");
  const [attachments, setAttachments] = React.useState<UploadedAttachment[]>([]);
  const [uploading, setUploading] = React.useState(false);
  const [posting, setPosting] = React.useState(false);

  // Excluded contacts state for restricted audience
  const [showExclusions, setShowExclusions] = React.useState(false);
  const [contacts, setContacts] = React.useState<Contact[]>([]);
  const [loadingContacts, setLoadingContacts] = React.useState(false);
  const [excludedContactIds, setExcludedContactIds] = React.useState<string[]>([]);

  const fileInputRef = React.useRef<HTMLInputElement | null>(null);

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
      setError(
        cause instanceof ApiError ? cause.message : "Unable to load moments.",
      );
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

  // Prepend moments shared by friends while this view is open.
  React.useEffect(() => {
    const unsubscribe = subscribe((event) => {
      if (event.type !== "moment.created") return;
      const incoming = event.moment;
      setMoments((prev) =>
        prev.some((m) => m.id === incoming.id) ? prev : [incoming, ...prev],
      );
    });
    return unsubscribe;
  }, [subscribe]);

  // Load contacts when user opens the exclusions picker
  const handleToggleExclusions = async () => {
    const next = !showExclusions;
    setShowExclusions(next);
    if (next && contacts.length === 0) {
      setLoadingContacts(true);
      try {
        const res = await contactsApi.list();
        setContacts(res.items);
      } catch (cause) {
        toast({
          title: "Could not load contacts",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      } finally {
        setLoadingContacts(false);
      }
    }
  };

  const toggleExcludeContact = (contactUserId: string) => {
    setExcludedContactIds((prev) =>
      prev.includes(contactUserId)
        ? prev.filter((id) => id !== contactUserId)
        : [...prev, contactUserId],
    );
  };

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (attachments.length >= MAX_MEDIA) {
      toast({
        title: "Too many attachments",
        description: `You can attach up to ${MAX_MEDIA} files.`,
        variant: "error",
      });
      return;
    }
    setUploading(true);
    try {
      const uploaded = await momentsApi.uploadMedia(file);
      const isImage = uploaded.content_type.startsWith("image/");
      const isVideo = uploaded.content_type.startsWith("video/");
      if (!isImage && !isVideo) {
        toast({
          title: "Unsupported file",
          description: "Moments support photos and videos only.",
          variant: "error",
        });
        return;
      }
      setAttachments((prev) => [...prev, uploaded]);
    } catch (cause) {
      toast({
        title: "Upload failed",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setUploading(false);
    }
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = body.trim();
    if (!trimmed && attachments.length === 0) return;
    setPosting(true);
    try {
      const moment = await momentsApi.create({
        body: trimmed,
        visibility,
        excluded_user_ids:
          visibility === "friends" && excludedContactIds.length > 0
            ? excludedContactIds
            : undefined,
        media: attachments.map((a) => ({
          url: a.url,
          media_type: a.content_type.startsWith("video/") ? "video" : "image",
        })),
      });
      setMoments((prev) => [moment, ...prev]);
      setBody("");
      setAttachments([]);
      setExcludedContactIds([]);
      setShowExclusions(false);
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
      <div className="flex flex-col gap-4">
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

  const canPost = body.trim().length > 0 || attachments.length > 0;

  return (
    <div className="flex flex-col gap-4">
      <form
        onSubmit={handleSubmit}
        className="border-border-subtle bg-surface-raised rounded-xl border p-4"
      >
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="What's on your mind?"
          rows={3}
          aria-label="Moment body"
          className="border-border-subtle bg-surface w-full resize-none rounded-lg border px-3 py-2 text-sm"
        />

        {attachments.length > 0 ? (
          <ul className="mt-3 flex flex-wrap gap-2">
            {attachments.map((a) => (
              <li
                key={a.url}
                className="border-border-subtle bg-surface flex items-center gap-2 rounded-lg border px-2 py-1 text-xs"
              >
                <span className="max-w-40 truncate">{a.name}</span>
                <button
                  type="button"
                  aria-label={`Remove ${a.name}`}
                  onClick={() =>
                    setAttachments((prev) =>
                      prev.filter((item) => item.url !== a.url),
                    )
                  }
                  className="text-ink-muted hover:text-ink"
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        ) : null}

        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*,video/*"
              className="hidden"
              onChange={handleUpload}
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              loading={uploading}
              onClick={() => fileInputRef.current?.click()}
            >
              Add photo / video
            </Button>
            <select
              value={visibility}
              onChange={(e) => setVisibility(e.target.value as Visibility)}
              aria-label="Visibility"
              className="border-border-subtle bg-surface text-ink rounded-md border px-2 py-1 text-sm"
            >
              <option value="friends">Friends</option>
              <option value="public">Public</option>
              <option value="private">Only me</option>
            </select>
            {visibility === "friends" ? (
              <button
                type="button"
                onClick={() => void handleToggleExclusions()}
                className="text-ink-muted hover:text-ink text-xs underline"
              >
                {excludedContactIds.length > 0
                  ? `Exclude ${excludedContactIds.length} contact${excludedContactIds.length > 1 ? "s" : ""}`
                  : "Exclude contacts"}
              </button>
            ) : null}
          </div>
          <Button type="submit" loading={posting} disabled={!canPost}>
            Share
          </Button>
        </div>

        {/* Exclusions Picker */}
        {showExclusions && visibility === "friends" ? (
          <div className="border-border-subtle bg-surface mt-3 rounded-lg border p-3 text-xs">
            <p className="text-ink font-semibold mb-2">
              Select contacts to exclude from seeing this moment:
            </p>
            {loadingContacts ? (
              <p className="text-ink-muted">Loading contacts…</p>
            ) : contacts.length === 0 ? (
              <p className="text-ink-muted">No contacts found to exclude.</p>
            ) : (
              <div className="max-h-36 overflow-y-auto flex flex-col gap-1.5">
                {contacts.map((contact) => (
                  <label
                    key={contact.user.id}
                    className="flex items-center gap-2 cursor-pointer hover:text-ink text-ink-muted"
                  >
                    <input
                      type="checkbox"
                      checked={excludedContactIds.includes(contact.user.id)}
                      onChange={() => toggleExcludeContact(contact.user.id)}
                      className="rounded border-border-subtle"
                    />
                    <span>{contact.user.display_name}</span>
                    {contact.user.username ? (
                      <span className="text-ink-muted opacity-70">
                        (@{contact.user.username})
                      </span>
                    ) : null}
                  </label>
                ))}
              </div>
            )}
          </div>
        ) : null}
      </form>

      {moments.length === 0 ? (
        <EmptyState
          title="No moments yet"
          description="When your friends share, they'll show up here."
        />
      ) : (
        <MomentStack
          moments={moments}
          setMoments={setMoments}
          currentUserId={currentUserId}
        />
      )}

      {nextCursor ? (
        <div className="flex justify-center">
          <Button
            variant="ghost"
            size="sm"
            loading={loadingMore}
            onClick={() => void fetchFeed(nextCursor)}
          >
            Load more
          </Button>
        </div>
      ) : null}
    </div>
  );
}
