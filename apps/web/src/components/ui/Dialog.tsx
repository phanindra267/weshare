"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  className?: string;
}

/**
 * Accessible modal built on the native <dialog> element.
 *
 * Native dialogs provide focus trapping, Escape-to-close, and inert
 * background content for free. We only add a click-outside handler and
 * controlled open state.
 */
export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  className,
}: DialogProps) {
  const ref = React.useRef<HTMLDialogElement>(null);
  const titleId = React.useId();
  const descriptionId = React.useId();

  React.useEffect(() => {
    const node = ref.current;
    if (!node) return;
    if (open && !node.open) node.showModal();
    if (!open && node.open) node.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        // Clicking the backdrop (the dialog element itself) closes the modal.
        if (event.target === ref.current) onClose();
      }}
      className={cn(
        "border-border-subtle bg-surface-raised text-ink w-[min(32rem,calc(100vw-2rem))] rounded-xl border p-0 shadow-xl",
        "backdrop:bg-black/40",
        className,
      )}
    >
      <div className="p-5">
        <h2 id={titleId} className="text-lg font-semibold">
          {title}
        </h2>
        {description ? (
          <p id={descriptionId} className="text-ink-muted mt-1 text-sm">
            {description}
          </p>
        ) : null}
        {children ? <div className="mt-4">{children}</div> : null}
      </div>
      {footer ? (
        <div className="border-border-subtle flex justify-end gap-2 border-t p-4">
          {footer}
        </div>
      ) : null}
    </dialog>
  );
}

export function DialogCloseButton({
  onClose,
  label = "Close dialog",
}: {
  onClose: () => void;
  label?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClose}
      aria-label={label}
      className="text-ink-muted hover:bg-surface-muted hover:text-ink focus-visible:outline-brand-600 absolute top-4 right-4 rounded-md p-1 focus-visible:outline-2"
    >
      <span aria-hidden="true">&times;</span>
    </button>
  );
}
