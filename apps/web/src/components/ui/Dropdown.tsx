"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface DropdownItem {
  id: string;
  label: string;
  onSelect: () => void;
  destructive?: boolean;
  disabled?: boolean;
}

export interface DropdownProps {
  trigger: React.ReactNode;
  items: DropdownItem[];
  align?: "start" | "end";
  label: string;
  className?: string;
}

/**
 * Small accessible dropdown menu.
 *
 * Implements roving focus with ArrowUp/ArrowDown, Home/End, Enter/Space to
 * activate, Escape to close, and click-outside dismissal.
 */
export function Dropdown({
  trigger,
  items,
  align = "end",
  label,
  className,
}: DropdownProps) {
  const [open, setOpen] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(0);
  const rootRef = React.useRef<HTMLDivElement>(null);
  const itemRefs = React.useRef<Array<HTMLButtonElement | null>>([]);

  React.useEffect(() => {
    if (!open) return;
    function onPointerDown(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  React.useEffect(() => {
    if (open) itemRefs.current[activeIndex]?.focus();
  }, [open, activeIndex]);

  const enabled = items
    .map((item, index) => ({ item, index }))
    .filter(({ item }) => !item.disabled)
    .map(({ index }) => index);

  function move(delta: number) {
    if (enabled.length === 0) return;
    const currentPos = enabled.indexOf(activeIndex);
    const nextPos = (currentPos + delta + enabled.length) % enabled.length;
    setActiveIndex(enabled[nextPos]);
  }

  return (
    <div ref={rootRef} className={cn("relative inline-block", className)}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        onClick={() => setOpen((v) => !v)}
        className="text-ink-muted hover:bg-surface-muted hover:text-ink focus-visible:outline-brand-600 inline-flex items-center justify-center rounded-md p-1.5 focus-visible:outline-2"
      >
        {trigger}
      </button>
      {open ? (
        <div
          role="menu"
          aria-label={label}
          onKeyDown={(event) => {
            if (event.key === "ArrowDown") {
              event.preventDefault();
              move(1);
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              move(-1);
            } else if (event.key === "Home") {
              event.preventDefault();
              setActiveIndex(enabled[0] ?? 0);
            } else if (event.key === "End") {
              event.preventDefault();
              setActiveIndex(enabled[enabled.length - 1] ?? 0);
            }
          }}
          className={cn(
            "border-border-subtle bg-surface-raised absolute z-50 mt-1 min-w-44 overflow-hidden rounded-md border py-1 shadow-lg",
            align === "end" ? "right-0" : "left-0",
          )}
        >
          {items.map((item, index) => (
            <button
              key={item.id}
              ref={(node) => {
                itemRefs.current[index] = node;
              }}
              role="menuitem"
              type="button"
              disabled={item.disabled}
              tabIndex={index === activeIndex ? 0 : -1}
              onMouseEnter={() => !item.disabled && setActiveIndex(index)}
              onClick={() => {
                setOpen(false);
                item.onSelect();
              }}
              className={cn(
                "hover:bg-surface-muted focus-visible:bg-surface-muted block w-full px-3 py-2 text-left text-sm focus-visible:outline-none disabled:opacity-50",
                item.destructive ? "text-red-600" : "text-ink",
              )}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
