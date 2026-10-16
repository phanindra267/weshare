"use client";

import * as React from "react";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Skeleton } from "@/components/ui/Skeleton";
import type { HealthResponse, ReadyResponse } from "@/types/api";

type State =
  | { kind: "loading" }
  | { kind: "ok"; health: HealthResponse; ready: ReadyResponse }
  | { kind: "degraded"; health: HealthResponse; error: string }
  | { kind: "error"; message: string };

function isAbort(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === "AbortError";
}

function describe(cause: unknown, fallback: string): string {
  return cause instanceof ApiError ? cause.message : fallback;
}

/**
 * Live connectivity probe against the Go API.
 *
 * Calls `/healthz` and `/readyz` and reports the observed result, including
 * database readiness. No fake "connected" state is ever shown.
 */
export function BackendStatus() {
  const [state, setState] = React.useState<State>({ kind: "loading" });

  // State is only updated inside promise callbacks (never synchronously in
  // the effect body), which avoids cascading renders.
  const fetchStatus = React.useCallback(
    (signal: AbortSignal): Promise<void> =>
      api
        .get<HealthResponse>("/healthz", { signal })
        .then((health) =>
          api
            .get<ReadyResponse>("/readyz", { signal })
            .then((ready) => {
              setState({ kind: "ok", health, ready });
            })
            .catch((cause: unknown) => {
              if (isAbort(cause)) return;
              setState({
                kind: "degraded",
                health,
                error: describe(cause, "Database readiness check failed."),
              });
            }),
        )
        .catch((cause: unknown) => {
          if (isAbort(cause)) return;
          setState({
            kind: "error",
            message: describe(cause, "Unable to reach the API."),
          });
        }),
    [],
  );

  React.useEffect(() => {
    const controller = new AbortController();
    void fetchStatus(controller.signal);
    return () => controller.abort();
  }, [fetchStatus]);

  const retry = () => {
    setState({ kind: "loading" });
    void fetchStatus(new AbortController().signal);
  };

  if (state.kind === "loading") {
    return (
      <div
        className="space-y-2"
        aria-busy="true"
        aria-label="Checking backend status"
      >
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-4 w-32" />
      </div>
    );
  }

  const dot = (color: string) => (
    <span
      aria-hidden="true"
      className={`inline-block size-2.5 rounded-full ${color}`}
    />
  );

  if (state.kind === "error") {
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm text-red-700 dark:text-red-300">
          {dot("bg-red-500")}
          API unreachable
        </p>
        <p className="text-ink-muted text-xs">{state.message}</p>
        <Button size="sm" variant="secondary" onClick={retry}>
          Retry
        </Button>
      </div>
    );
  }

  if (state.kind === "degraded") {
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm text-amber-700 dark:text-amber-300">
          {dot("bg-amber-500")}
          API up, database not ready
        </p>
        <p className="text-ink-muted text-xs">{state.error}</p>
        <Button size="sm" variant="secondary" onClick={retry}>
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-1.5">
      <p className="text-brand-700 dark:text-brand-300 flex items-center gap-2 text-sm">
        {dot("bg-brand-500")}
        API and database ready
      </p>
      <p className="text-ink-muted text-xs">
        service <code className="font-mono">{state.health.service}</code> · v
        {state.health.version} · uptime{" "}
        {Math.floor(state.health.uptime_seconds)}s
      </p>
    </div>
  );
}
