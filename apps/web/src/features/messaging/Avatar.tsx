"use client";

import { cn } from "@/lib/cn";
import { mediaUrl } from "@/lib/media";
import { initials } from "@/features/messaging/format";

export interface AvatarProps {
  name: string;
  avatarUrl: string | null;
  /** Tailwind size classes for the square/circle; default "size-10". */
  className?: string;
}

/** Circular avatar that falls back to initials when no image is set. */
export function Avatar({ name, avatarUrl, className }: AvatarProps) {
  const src = mediaUrl(avatarUrl);
  return src ? (
    <img
      src={src}
      alt=""
      className={cn(
        "bg-surface-overlay size-10 shrink-0 rounded-full object-cover",
        className,
      )}
    />
  ) : (
    <span
      aria-hidden="true"
      className={cn(
        "bg-surface-overlay text-ink-muted grid size-10 shrink-0 place-items-center rounded-full text-sm font-medium",
        className,
      )}
    >
      {initials(name)}
    </span>
  );
}
