import type { Metadata } from "next";
import Link from "next/link";
import { RegisterForm } from "@/features/auth/RegisterForm";

export const metadata: Metadata = {
  title: "Create account",
};

export default function RegisterPage() {
  return (
    <main id="main" className="mx-auto max-w-md px-6 py-16">
      <h1 className="text-2xl font-semibold tracking-tight">
        Create your account
      </h1>
      <p className="text-ink-muted mt-1 text-sm">
        Join WeShare in a few seconds.
      </p>

      <div className="mt-8">
        <RegisterForm />
      </div>

      <p className="text-ink-muted mt-6 text-sm">
        Already have an account?{" "}
        <Link href="/login" className="text-brand-700 dark:text-brand-300 hover:underline">
          Sign in
        </Link>
      </p>
    </main>
  );
}
