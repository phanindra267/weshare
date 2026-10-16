"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  invalid?: boolean;
}

export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  function Select({ className, invalid, children, ...props }, ref) {
    return (
      <select
        ref={ref}
        aria-invalid={invalid || undefined}
        className={cn(
          "bg-surface-raised text-ink h-10 w-full rounded-md border px-3 text-sm",
          "focus-visible:outline-brand-600 focus-visible:outline-2 focus-visible:outline-offset-0",
          "disabled:cursor-not-allowed disabled:opacity-60",
          invalid ? "border-red-500" : "border-border-subtle",
          className,
        )}
        {...props}
      >
        {children}
      </select>
    );
  },
);
