"use client";

import Link from "next/link";
import { Button } from "@/components/ui/Button";
import { useAuth } from "@/features/auth/AuthProvider";

/**
 * Top navigation that reflects the current session. Renders nothing until the
 * session resolves to avoid flicker between anon/authed states.
 */
export function AppHeader() {
  const { status, user, logout } = useAuth();

  return (
    <header className="border-border-subtle bg-surface-raised/80 sticky top-0 z-50 border-b backdrop-blur">
      <div className="mx-auto flex h-14 max-w-4xl items-center justify-between px-6">
        <Link
          href="/"
          className="text-ink flex items-center gap-2 font-semibold"
        >
          <span
            aria-hidden="true"
            className="bg-brand-600 grid size-7 place-items-center rounded-lg text-sm text-white"
          >
            W
          </span>
          WeShare
        </Link>

        <nav className="flex items-center gap-2">
          {status === "authenticated" && user ? (
            <>
              <Link
                href="/messages"
                className="text-ink-muted hover:text-ink text-sm"
              >
                Messages
              </Link>
              <Link
                href="/contacts"
                className="text-ink-muted hover:text-ink text-sm"
              >
                Contacts
              </Link>
              <Link
                href="/profile"
                className="text-ink-muted hover:text-ink text-sm"
              >
                {user.display_name}
              </Link>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void logout()}
              >
                Sign out
              </Button>
            </>
          ) : status === "anonymous" ? (
            <>
              <Link
                href="/login"
                className="text-ink-muted hover:text-ink text-sm"
              >
                Sign in
              </Link>
              <Link href="/register">
                <Button size="sm">Create account</Button>
              </Link>
            </>
          ) : null}
        </nav>
      </div>
    </header>
  );
}
