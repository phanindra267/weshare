"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { useToast } from "@/components/ui/Toast";
import { ApiError } from "@/lib/api";
import { fieldErrorsFrom } from "@/features/auth/api";
import type { FieldErrors } from "@/features/auth/types";
import { messagingApi } from "@/features/messaging/api";
import { MemberPicker } from "@/features/messaging/MemberPicker";
import type { Conversation } from "@/features/messaging/types";

export interface CreateGroupDialogProps {
  open: boolean;
  onClose: () => void;
  /** Called with the created group so the caller can select it. */
  onCreated: (conversation: Conversation) => void;
}

/**
 * Modal for creating a group: a name plus an initial set of contacts. The
 * creator becomes the owner on the server; at least one other member is
 * required so the group starts as a real conversation.
 */
export function CreateGroupDialog({
  open,
  onClose,
  onCreated,
}: CreateGroupDialogProps) {
  const { toast } = useToast();
  const [title, setTitle] = React.useState("");
  const [selected, setSelected] = React.useState<string[]>([]);
  const [fields, setFields] = React.useState<FieldErrors>({});
  const [submitting, setSubmitting] = React.useState(false);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFields({});
    setSubmitting(true);
    try {
      const conversation = await messagingApi.createGroup({
        title: title.trim(),
        member_ids: selected,
      });
      toast({ title: "Group created", variant: "success" });
      onCreated(conversation);
      onClose();
    } catch (cause) {
      const fieldErrors = fieldErrorsFrom(cause);
      if (Object.keys(fieldErrors).length > 0) {
        setFields(fieldErrors);
      } else {
        toast({
          title: "Could not create group",
          description: cause instanceof ApiError ? cause.message : undefined,
          variant: "error",
        });
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New group"
      description="Name your group and choose who to add."
      footer={
        <>
          <Button variant="secondary" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="create-group-form" loading={submitting}>
            Create group
          </Button>
        </>
      }
    >
      <form id="create-group-form" onSubmit={submit} className="space-y-4">
        <Field label="Group name" htmlFor="group-title" error={fields.title}>
          <Input
            id="group-title"
            value={title}
            maxLength={64}
            placeholder="Weekend crew"
            invalid={Boolean(fields.title)}
            onChange={(e) => setTitle(e.target.value)}
          />
        </Field>
        <div>
          <p className="text-ink mb-2 text-sm font-medium">Members</p>
          <MemberPicker
            idPrefix="create-group"
            selected={selected}
            onChange={setSelected}
          />
          {fields.member_ids ? (
            <p role="alert" className="mt-1 text-xs text-red-600">
              {fields.member_ids}
            </p>
          ) : null}
        </div>
      </form>
    </Dialog>
  );
}
