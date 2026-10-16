import { Suspense } from "react";
import type { Metadata } from "next";
import Link from "next/link";
import { LoginForm } from "@/features/auth/LoginForm";
import { Skeleton } from "@/components/ui/Skeleton";

export const metadata: Metadata = {
  title: "Sign in",
};

export default function LoginPage() {
  return (
    <main id="main" className="mx-auto max-w-md px-6 py-16">
      <h1 className="text-2xl font-semibold tracking-tight">Welcome back</h1>
      <p className="text-ink-muted mt-1 text-sm">
        Sign in to access your conversations.
      </p>

      <div className="mt-8">
        <Suspense
          fallback={
            <div className="space-y-3" aria-busy="true">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-32" />
            </div>
          }
        >
          <LoginForm />
        </Suspense>
      </div>

      <p className="text-ink-muted mt-6 text-sm">
        No account?{" "}
        <Link href="/register" className="text-brand-700 dark:text-brand-300 hover:underline">
          Create one
        </Link>
      </p>
    </main>
  );
}
