"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";

export function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { login } = useAuth();

  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [fields, setFields] = React.useState<FieldErrors>({});
  const [error, setError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    setFields({});
    try {
      await login({ email, password });
      const next = searchParams.get("next");
      router.push(next && next.startsWith("/") ? next : "/profile");
    } catch (cause) {
      const details = fieldErrorsFrom(cause);
      if (Object.keys(details).length > 0) {
        setFields(details);
      } else if (cause instanceof ApiError) {
        setError(cause.message);
      } else {
        setError("Unable to sign in. Please try again.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {error ? <ErrorMessage title="Sign in failed" message={error} /> : null}

      <Field label="Email" htmlFor="login-email" error={fields.email} required>
        <Input
          id="login-email"
          name="email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
      </Field>

      <Field
        label="Password"
        htmlFor="login-password"
        error={fields.password}
        required
      >
        <Input
          id="login-password"
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
      </Field>

      <Button type="submit" loading={submitting} className="mt-2">
        Sign in
      </Button>
    </form>
  );
}
