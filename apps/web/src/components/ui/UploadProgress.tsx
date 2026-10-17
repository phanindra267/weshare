"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface UploadProgressProps {
  /** 0–100 */
  percent: number;
  className?: string;
}

/** Animated progress bar shown during file upload. */
export function UploadProgress({ percent, className }: UploadProgressProps) {
  return (
    <div
      role="progressbar"
      aria-valuenow={percent}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={`Uploading: ${percent}%`}
      className={cn("h-1.5 w-full overflow-hidden rounded-full bg-surface-muted", className)}
    >
      <div
        className="h-full bg-brand-600 transition-[width] duration-150 ease-out"
        style={{ width: `${Math.max(0, Math.min(100, percent))}%` }}
      />
    </div>
  );
}
