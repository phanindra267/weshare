"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/features/auth/AuthProvider";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";

export function RegisterForm() {
  const router = useRouter();
  const { register } = useAuth();

  const [form, setForm] = React.useState({
    email: "",
    username: "",
    display_name: "",
    password: "",
  });
  const [fields, setFields] = React.useState<FieldErrors>({});
  const [error, setError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  const update =
    (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev) => ({ ...prev, [key]: e.target.value }));

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    setFields({});
    try {
      await register({
        email: form.email,
        username: form.username,
        password: form.password,
        display_name: form.display_name || undefined,
      });
      router.push("/profile");
    } catch (cause) {
      const details = fieldErrorsFrom(cause);
      if (Object.keys(details).length > 0) {
        setFields(details);
      } else if (cause instanceof ApiError) {
        setError(cause.message);
      } else {
        setError("Unable to create your account. Please try again.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {error ? (
        <ErrorMessage title="Registration failed" message={error} />
      ) : null}

      <Field label="Email" htmlFor="reg-email" error={fields.email} required>
        <Input
          id="reg-email"
          name="email"
          type="email"
          autoComplete="email"
          value={form.email}
          onChange={update("email")}
          required
        />
      </Field>

      <Field
        label="Username"
        htmlFor="reg-username"
        description="3-32 lowercase letters, numbers, or underscores."
        error={fields.username}
        required
      >
        <Input
          id="reg-username"
          name="username"
          autoComplete="username"
          value={form.username}
          onChange={update("username")}
          required
        />
      </Field>

      <Field
        label="Display name"
        htmlFor="reg-display-name"
        description="Optional. Defaults to your username."
        error={fields.display_name}
      >
        <Input
          id="reg-display-name"
          name="display_name"
          autoComplete="nickname"
          value={form.display_name}
          onChange={update("display_name")}
        />
      </Field>

      <Field
        label="Password"
        htmlFor="reg-password"
        description="At least 8 characters."
        error={fields.password}
        required
      >
        <Input
          id="reg-password"
          name="password"
          type="password"
          autoComplete="new-password"
          value={form.password}
          onChange={update("password")}
          required
        />
      </Field>

      <Button type="submit" loading={submitting} className="mt-2">
        Create account
      </Button>
    </form>
  );
}
