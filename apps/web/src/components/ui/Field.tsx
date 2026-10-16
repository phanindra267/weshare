"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface FieldProps {
  label: string;
  htmlFor: string;
  description?: string;
  error?: string;
  required?: boolean;
  className?: string;
  children: React.ReactNode;
}

/**
 * Accessible form field wrapper.
 *
 * The error text is rendered in a live region so screen readers announce
 * validation failures as they appear.
 */
export function Field({
  label,
  htmlFor,
  description,
  error,
  required,
  className,
  children,
}: FieldProps) {
  const descriptionId = description ? `${htmlFor}-description` : undefined;
  const errorId = error ? `${htmlFor}-error` : undefined;
  const describedBy =
    [descriptionId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={htmlFor} className="text-ink text-sm font-medium">
        {label}
        {required ? (
          <span aria-hidden="true" className="ml-0.5 text-red-500">
            *
          </span>
        ) : null}
      </label>
      {React.isValidElement(children)
        ? React.cloneElement(
            children as React.ReactElement<{
              "aria-describedby"?: string;
              "aria-required"?: boolean;
              invalid?: boolean;
            }>,
            {
              "aria-describedby": describedBy,
              "aria-required": required || undefined,
              invalid: Boolean(error),
            },
          )
        : children}
      {description ? (
        <p id={descriptionId} className="text-ink-muted text-xs">
          {description}
        </p>
      ) : null}
      <p
        id={errorId}
        role="alert"
        aria-live="polite"
        className={cn("text-xs text-red-600", error ? "" : "sr-only")}
      >
        {error}
      </p>
    </div>
  );
}
