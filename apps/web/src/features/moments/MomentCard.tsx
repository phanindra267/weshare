"use client";

import * as React from "react";
import type { Moment } from "./types";

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

interface MomentCardProps {
  moment: Moment;
}

export function MomentCard({ moment }: MomentCardProps) {
  return (
    <article className="border-border-subtle bg-surface-raised rounded-xl border p-4 shadow-sm">
      <header className="flex items-center justify-between gap-3">
        <div>
          <p className="text-ink text-sm font-semibold">{moment.author.display_name}</p>
          <p className="text-ink-muted text-xs">{formatTime(moment.created_at)}</p>
        </div>
        <span className="text-ink-muted text-xs capitalize">{moment.visibility}</span>
      </header>
      {moment.body ? (
        <p className="text-ink mt-3 whitespace-pre-wrap text-sm leading-relaxed">{moment.body}</p>
      ) : null}
      {moment.media.length > 0 ? (
        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {moment.media.map((media) => (
            <img
              key={media.id}
              src={media.url}
              alt="Moment attachment"
              className="border-border-subtle rounded-lg border object-cover"
            />
          ))}
        </div>
      ) : null}
      <footer className="text-ink-muted mt-3 flex items-center justify-between text-xs">
        <span>{moment.like_count} likes • {moment.comment_count} comments</span>
      </footer>
    </article>
  );
}
