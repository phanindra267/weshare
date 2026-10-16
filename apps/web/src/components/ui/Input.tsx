"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  function Input({ className, invalid, ...props }, ref) {
    return (
      <input
        ref={ref}
        aria-invalid={invalid || undefined}
        className={cn(
          "bg-surface-raised text-ink h-10 w-full rounded-md border px-3 text-sm",
          "placeholder:text-ink-muted",
          "focus-visible:outline-brand-600 focus-visible:outline-2 focus-visible:outline-offset-0",
          "disabled:cursor-not-allowed disabled:opacity-60",
          invalid ? "border-red-500" : "border-border-subtle",
          className,
        )}
        {...props}
      />
    );
  },
);

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean;
}

export const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  function Textarea({ className, invalid, ...props }, ref) {
    return (
      <textarea
        ref={ref}
        aria-invalid={invalid || undefined}
        className={cn(
          "bg-surface-raised text-ink w-full rounded-md border px-3 py-2 text-sm",
          "placeholder:text-ink-muted",
          "focus-visible:outline-brand-600 focus-visible:outline-2 focus-visible:outline-offset-0",
          "disabled:cursor-not-allowed disabled:opacity-60",
          invalid ? "border-red-500" : "border-border-subtle",
          className,
        )}
        {...props}
      />
    );
  },
);
