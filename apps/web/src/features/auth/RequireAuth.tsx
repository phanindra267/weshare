"use client";

import * as React from "react";
import { usePathname, useRouter } from "next/navigation";
import { Skeleton } from "@/components/ui/Skeleton";
import { useAuth } from "@/features/auth/AuthProvider";

/**
 * Client-side gate for authenticated routes.
 *
 * While the session is resolving it shows a skeleton (never a flash of the
 * protected UI). Anonymous visitors are redirected to `/login` with a
 * `next` parameter so they return here after signing in.
 */
export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  React.useEffect(() => {
    if (status === "anonymous") {
      const next = encodeURIComponent(pathname || "/");
      router.replace(`/login?next=${next}`);
    }
  }, [status, router, pathname]);

  if (status !== "authenticated") {
    return (
      <div className="space-y-3" aria-busy="true" aria-label="Loading account">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-4 w-64" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  return <>{children}</>;
}
