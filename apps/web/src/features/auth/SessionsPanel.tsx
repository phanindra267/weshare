"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { EmptyState } from "@/components/ui/EmptyState";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { authApi } from "@/features/auth/api";
import type { Session } from "@/features/auth/types";

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

type State =
  | { kind: "loading" }
  | { kind: "ready"; sessions: Session[] }
  | { kind: "error"; message: string };

export function SessionsPanel() {
  const { toast } = useToast();
  const [state, setState] = React.useState<State>({ kind: "loading" });
  const [busyId, setBusyId] = React.useState<string | null>(null);

  // State is updated only inside promise callbacks (never synchronously in an
  // effect body), which avoids cascading renders.
  const load = React.useCallback(
    (): Promise<void> =>
      authApi
        .listSessions()
        .then((page) => {
          setState({ kind: "ready", sessions: page.items });
        })
        .catch((cause: unknown) => {
          setState({
            kind: "error",
            message:
              cause instanceof ApiError
                ? cause.message
                : "Unable to load your sessions.",
          });
        }),
    [],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  const revoke = async (id: string) => {
    setBusyId(id);
    try {
      await authApi.revokeSession(id);
      toast({ title: "Session revoked", variant: "success" });
      await load();
    } catch (cause) {
      toast({
        title: "Could not revoke session",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setBusyId(null);
    }
  };

  const revokeOthers = async () => {
    setBusyId("all");
    try {
      await authApi.revokeOtherSessions();
      toast({ title: "Other sessions signed out", variant: "success" });
      await load();
    } catch (cause) {
      toast({
        title: "Could not sign out other sessions",
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setBusyId(null);
    }
  };

  if (state.kind === "loading") {
    return (
      <div className="space-y-3" aria-busy="true" aria-label="Loading sessions">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }

  if (state.kind === "error") {
    return (
      <ErrorMessage
        title="Could not load sessions"
        message={state.message}
        onRetry={() => {
          setState({ kind: "loading" });
          void load();
        }}
      />
    );
  }

  if (state.sessions.length === 0) {
    return <EmptyState title="No active sessions" />;
  }

  const hasOthers = state.sessions.some((s) => !s.is_current);

  return (
    <div className="flex flex-col gap-4">
      <ul className="flex flex-col gap-2">
        {state.sessions.map((session) => (
          <li
            key={session.id}
            className="border-border-subtle bg-surface-raised flex items-start justify-between gap-3 rounded-lg border p-3"
          >
            <div className="min-w-0">
              <p className="text-ink truncate text-sm font-medium">
                {session.user_agent || "Unknown device"}
                {session.is_current ? (
                  <span className="bg-brand-600/15 text-brand-700 dark:text-brand-300 ml-2 rounded px-1.5 py-0.5 text-xs">
                    This device
                  </span>
                ) : null}
              </p>
              <p className="text-ink-muted mt-0.5 text-xs">
                {session.ip ? `${session.ip} · ` : ""}Last used{" "}
                {formatDate(session.last_used_at)}
              </p>
            </div>
            {!session.is_current ? (
              <Button
                variant="secondary"
                size="sm"
                loading={busyId === session.id}
                onClick={() => void revoke(session.id)}
              >
                Revoke
              </Button>
            ) : null}
          </li>
        ))}
      </ul>

      {hasOthers ? (
        <Button
          variant="secondary"
          size="sm"
          className="self-start"
          loading={busyId === "all"}
          onClick={() => void revokeOthers()}
        >
          Sign out other devices
        </Button>
      ) : null}
    </div>
  );
}
