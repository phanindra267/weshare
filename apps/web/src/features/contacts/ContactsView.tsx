"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { ErrorMessage } from "@/components/ui/ErrorMessage";
import { EmptyState } from "@/components/ui/EmptyState";
import { Skeleton } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { mediaUrl } from "@/lib/media";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";
import { contactsApi } from "@/features/contacts/api";
import type {
  Contact,
  ContactRequest,
  UserBrief,
} from "@/features/contacts/types";
import { ContactCallButtons } from "@/features/calls/ContactCallButtons";

interface Data {
  contacts: Contact[];
  incoming: ContactRequest[];
  outgoing: ContactRequest[];
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "?";
}

function Avatar({ user }: { user: UserBrief }) {
  const src = mediaUrl(user.avatar_url);
  return (
    <span className="relative inline-flex shrink-0">
      {src ? (
        <img
          src={src}
          alt=""
          className="bg-surface-overlay size-9 rounded-full object-cover"
        />
      ) : (
        <span
          aria-hidden="true"
          className="bg-surface-overlay text-ink-muted grid size-9 place-items-center rounded-full text-xs font-medium"
        >
          {initials(user.display_name)}
        </span>
      )}
      {user.online ? (
        <span
          title="Online"
          className="ring-surface-raised absolute -right-0.5 -bottom-0.5 size-3 rounded-full bg-green-500 ring-2"
        />
      ) : null}
    </span>
  );
}

function UserRow({
  user,
  children,
}: {
  user: UserBrief;
  children?: React.ReactNode;
}) {
  return (
    <li className="border-border-subtle bg-surface-raised flex items-center justify-between gap-3 rounded-lg border p-3">
      <div className="flex min-w-0 items-center gap-3">
        <Avatar user={user} />
        <div className="min-w-0">
          <p className="text-ink truncate text-sm font-medium">
            {user.display_name}
          </p>
          <p className="text-ink-muted truncate text-xs">
            {user.status_message ??
              (user.username ? `@${user.username}` : "No status")}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">{children}</div>
    </li>
  );
}

export function ContactsView() {
  const { toast } = useToast();
  const [data, setData] = React.useState<Data | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState<string | null>(null);

  const [addId, setAddId] = React.useState("");
  const [addFields, setAddFields] = React.useState<FieldErrors>({});
  const [adding, setAdding] = React.useState(false);

  // State updates happen only inside promise callbacks.
  const load = React.useCallback(
    (): Promise<void> =>
      Promise.all([
        contactsApi.list(),
        contactsApi.listRequests("incoming"),
        contactsApi.listRequests("outgoing"),
      ])
        .then(([contacts, incoming, outgoing]) => {
          setData({
            contacts: contacts.items,
            incoming: incoming.items,
            outgoing: outgoing.items,
          });
          setError(null);
        })
        .catch((cause: unknown) => {
          setError(
            cause instanceof ApiError
              ? cause.message
              : "Unable to load your contacts.",
          );
        }),
    [],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  const run = async (
    key: string,
    action: () => Promise<unknown>,
    successTitle: string,
    failureTitle: string,
  ) => {
    setBusy(key);
    try {
      await action();
      toast({ title: successTitle, variant: "success" });
      await load();
    } catch (cause) {
      toast({
        title: failureTitle,
        description: cause instanceof ApiError ? cause.message : undefined,
        variant: "error",
      });
    } finally {
      setBusy(null);
    }
  };



  const submitForm = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setAddFields({});
    setAdding(true);
    try {
      const result = await contactsApi.sendRequest(addId.trim());
      toast({
        title:
          result.status === "accepted"
            ? "You are now connected"
            : "Contact request sent",
        variant: "success",
      });
      setAddId("");
      await load();
    } catch (cause) {
      const fields = fieldErrorsFrom(cause);
      if (Object.keys(fields).length > 0) {
        setAddFields(fields);
      } else {
        toast({
          title: "Could not send request",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      }
    } finally {
      setAdding(false);
    }
  };

  if (error && !data) {
    return (
      <ErrorMessage
        title="Could not load contacts"
        message={error}
        onRetry={() => {
          setError(null);
          void load();
        }}
      />
    );
  }

  if (!data) {
    return (
      <div className="space-y-3" aria-busy="true" aria-label="Loading contacts">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <section className="border-border-subtle bg-surface-raised rounded-xl border p-6">
        <h2 className="text-base font-semibold">Add a contact</h2>
        <p className="text-ink-muted mt-1 text-sm">
          Enter a user&apos;s id to send a friend request.
        </p>
        <form className="mt-4 flex items-start gap-2" onSubmit={submitForm}>
          <Field
            label="User id"
            htmlFor="contact-user-id"
            error={addFields.user_id}
            className="flex-1"
          >
            <Input
              id="contact-user-id"
              value={addId}
              placeholder="00000000-0000-0000-0000-000000000000"
              autoComplete="off"
              onChange={(e) => setAddId(e.target.value)}
            />
          </Field>
          <Button type="submit" loading={adding} className="mt-6">
            Send request
          </Button>
        </form>
      </section>

      <section className="border-border-subtle bg-surface-raised rounded-xl border p-6">
        <h2 className="text-base font-semibold">
          Friend requests
          {data.incoming.length > 0 ? (
            <span className="bg-brand-600/15 text-brand-700 dark:text-brand-300 ml-2 rounded px-1.5 py-0.5 text-xs">
              {data.incoming.length}
            </span>
          ) : null}
        </h2>
        <div className="mt-4">
          {data.incoming.length === 0 ? (
            <EmptyState title="No incoming requests" />
          ) : (
            <ul className="flex flex-col gap-2">
              {data.incoming.map((req) => (
                <UserRow key={req.id} user={req.user}>
                  <Button
                    size="sm"
                    loading={busy === `accept:${req.id}`}
                    onClick={() =>
                      void run(`accept:${req.id}`, () => contactsApi.accept(req.id), "Request accepted", "Could not accept request")
                    }
                  >
                    Accept
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={busy === `decline:${req.id}`}
                    onClick={() =>
                      void run(`decline:${req.id}`, () => contactsApi.decline(req.id), "Request declined", "Could not decline request")
                    }
                  >
                    Decline
                  </Button>
                </UserRow>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section className="border-border-subtle bg-surface-raised rounded-xl border p-6">
        <h2 className="text-base font-semibold">
          Contacts
          {data.contacts.length > 0 ? (
            <span className="text-ink-muted ml-2 text-xs">
              {data.contacts.length}
            </span>
          ) : null}
        </h2>
        <div className="mt-4">
          {data.contacts.length === 0 ? (
            <EmptyState
              title="No contacts yet"
              description="Send a friend request to start connecting."
            />
          ) : (
            <ul className="flex flex-col gap-2">
              {data.contacts.map((c) => (
                <UserRow key={c.user.id} user={c.user}>
                  <ContactCallButtons contact={c.user} />
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={busy === `remove:${c.user.id}`}
                    onClick={() =>
                      void run(`remove:${c.user.id}`, () => contactsApi.remove(c.user.id), "Contact removed", "Could not remove contact")
                    }
                  >
                    Remove
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    loading={busy === `block:${c.user.id}`}
                    onClick={() =>
                      void run(`block:${c.user.id}`, () => contactsApi.block(c.user.id), "User blocked", "Could not block user")
                    }
                  >
                    Block
                  </Button>
                </UserRow>
              ))}
            </ul>
          )}
        </div>
      </section>

      {data.outgoing.length > 0 ? (
        <section className="border-border-subtle bg-surface-raised rounded-xl border p-6">
          <h2 className="text-base font-semibold">Sent requests</h2>
          <ul className="mt-4 flex flex-col gap-2">
            {data.outgoing.map((req) => (
              <UserRow key={req.id} user={req.user}>
                <span className="text-ink-muted text-xs">Pending</span>
              </UserRow>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
