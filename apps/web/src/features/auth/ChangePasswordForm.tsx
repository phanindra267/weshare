"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { authApi, fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";

export function ChangePasswordForm() {
  const { toast } = useToast();
  const [current, setCurrent] = React.useState("");
  const [next, setNext] = React.useState("");
  const [fields, setFields] = React.useState<FieldErrors>({});
  const [error, setError] = React.useState<string | null>(null);
  const [submitting, setSubmitting] = React.useState(false);

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    setFields({});
    try {
      await authApi.changePassword(current, next);
      setCurrent("");
      setNext("");
      toast({
        title: "Password changed",
        description: "Other devices have been signed out.",
        variant: "success",
      });
    } catch (cause) {
      const details = fieldErrorsFrom(cause);
      if (Object.keys(details).length > 0) setFields(details);
      else if (cause instanceof ApiError) setError(cause.message);
      else setError("Unable to change your password. Please try again.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {error ? (
        <ErrorMessage title="Password change failed" message={error} />
      ) : null}

      <Field
        label="Current password"
        htmlFor="pw-current"
        error={fields.current_password}
        required
      >
        <Input
          id="pw-current"
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          required
        />
      </Field>

      <Field
        label="New password"
        htmlFor="pw-new"
        description="At least 8 characters."
        error={fields.new_password}
        required
      >
        <Input
          id="pw-new"
          type="password"
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          required
        />
      </Field>

      <Button type="submit" loading={submitting} className="self-start">
        Update password
      </Button>
    </form>
  );
}
