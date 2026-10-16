"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input, Textarea } from "@/components/ui/Input";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { mediaUrl } from "@/lib/media";
import { profileApi, type Profile } from "@/features/profile/api";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";

const MAX_AVATAR_BYTES = 2 * 1024 * 1024;

export function ProfileEditor({ initial }: { initial?: Profile } = {}) {
  const { toast } = useToast();

  const [profile, setProfile] = React.useState<Profile | null>(initial ?? null);
  const [loading, setLoading] = React.useState(!initial);
  const [loadError, setLoadError] = React.useState<string | null>(null);

  const [form, setForm] = React.useState({
    display_name: initial?.display_name ?? "",
    bio: initial?.bio ?? "",
    status_message: initial?.status_message ?? "",
  });
  const [fields, setFields] = React.useState<FieldErrors>({});
  const [error, setError] = React.useState<string | null>(null);
  const [saving, setSaving] = React.useState(false);
  const [uploading, setUploading] = React.useState(false);

  const applyProfile = React.useCallback((p: Profile) => {
    setProfile(p);
    setForm({
      display_name: p.display_name,
      bio: p.bio ?? "",
      status_message: p.status_message ?? "",
    });
  }, []);

  React.useEffect(() => {
    if (initial) return;
    let active = true;
    profileApi
      .me()
      .then((p) => {
        if (active) {
          applyProfile(p);
          setLoadError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!active) return;
        setLoadError(
          cause instanceof ApiError
            ? cause.message
            : "Unable to load your profile.",
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [initial, applyProfile]);

  const onSave = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setFields({});
    try {
      const updated = await profileApi.update({
        display_name: form.display_name,
        bio: form.bio,
        status_message: form.status_message,
      });
      applyProfile(updated);
      toast({ title: "Profile saved", variant: "success" });
    } catch (cause) {
      const details = fieldErrorsFrom(cause);
      if (Object.keys(details).length > 0) setFields(details);
      else if (cause instanceof ApiError) setError(cause.message);
      else setError("Unable to save your profile. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  const onAvatarChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;

    if (!["image/jpeg", "image/png", "image/webp"].includes(file.type)) {
      setError("Use a JPEG, PNG, or WebP image.");
      return;
    }
    if (file.size > MAX_AVATAR_BYTES) {
      setError("Image must be 2 MB or smaller.");
      return;
    }

    setUploading(true);
    setError(null);
    try {
      const updated = await profileApi.uploadAvatar(file);
      applyProfile(updated);
      toast({ title: "Avatar updated", variant: "success" });
    } catch (cause) {
      setError(
        cause instanceof ApiError ? cause.message : "Avatar upload failed.",
      );
    } finally {
      setUploading(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-3" aria-busy="true" aria-label="Loading profile">
        <Skeleton className="size-16 rounded-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  if (loadError) {
    return (
      <ErrorMessage
        title="Could not load profile"
        message={loadError}
        onRetry={() => {
          setLoading(true);
          setLoadError(null);
          profileApi
            .me()
            .then(applyProfile)
            .catch((cause: unknown) =>
              setLoadError(
                cause instanceof ApiError
                  ? cause.message
                  : "Unable to load your profile.",
              ),
            )
            .finally(() => setLoading(false));
        }}
      />
    );
  }

  const avatarSrc = mediaUrl(profile?.avatar_url);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-4">
        <div className="bg-surface-muted grid size-16 shrink-0 place-items-center overflow-hidden rounded-full">
          {avatarSrc ? (
            <img
              src={avatarSrc}
              alt="Your avatar"
              width={64}
              height={64}
              className="size-16 object-cover"
            />
          ) : (
            <span className="text-ink-muted text-lg font-semibold">
              {(profile?.display_name ?? "?").slice(0, 1).toUpperCase()}
            </span>
          )}
        </div>
        <div>
          <label
            htmlFor="avatar-upload"
            className="text-brand-700 dark:text-brand-300 cursor-pointer text-sm font-medium hover:underline"
          >
            {uploading ? "Uploading..." : "Change avatar"}
          </label>
          <input
            id="avatar-upload"
            type="file"
            accept="image/jpeg,image/png,image/webp"
            className="sr-only"
            onChange={onAvatarChange}
            disabled={uploading}
          />
          <p className="text-ink-muted text-xs">
            JPEG, PNG, or WebP up to 2 MB.
          </p>
        </div>
      </div>

      <form onSubmit={onSave} noValidate className="flex flex-col gap-4">
        {error ? <ErrorMessage title="Update failed" message={error} /> : null}

        <Field
          label="Display name"
          htmlFor="profile-display-name"
          error={fields.display_name}
          required
        >
          <Input
            id="profile-display-name"
            value={form.display_name}
            onChange={(e) =>
              setForm((f) => ({ ...f, display_name: e.target.value }))
            }
          />
        </Field>

        <Field
          label="Status"
          htmlFor="profile-status"
          description="A short message shown next to your name (max 140)."
          error={fields.status_message}
        >
          <Input
            id="profile-status"
            value={form.status_message}
            onChange={(e) =>
              setForm((f) => ({ ...f, status_message: e.target.value }))
            }
            maxLength={140}
          />
        </Field>

        <Field
          label="Bio"
          htmlFor="profile-bio"
          description="Tell others about yourself (max 500)."
          error={fields.bio}
        >
          <Textarea
            id="profile-bio"
            rows={4}
            value={form.bio}
            onChange={(e) => setForm((f) => ({ ...f, bio: e.target.value }))}
            maxLength={500}
          />
        </Field>

        <Button type="submit" loading={saving} className="self-start">
          Save changes
        </Button>
      </form>
    </div>
  );
}
