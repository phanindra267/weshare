"use client";

import * as React from "react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { conversationName } from "@/features/messaging/format";
import type { Conversation } from "@/features/messaging/types";

export interface ForwardDialogProps {
  open: boolean;
  /** Conversations the user can forward into. */
  conversations: Conversation[];
  /** The conversation the message came from; hidden from the target list. */
  sourceId: string;
  busy?: boolean;
  onClose: () => void;
  onForward: (conversationId: string) => void;
}

/**
 * Modal for choosing where to forward a message. Only conversations already in
 * the sidebar are offered; the origin conversation is excluded.
 */
export function ForwardDialog({
  open,
  conversations,
  sourceId,
  busy,
  onClose,
  onForward,
}: ForwardDialogProps) {
  const targets = conversations.filter((c) => c.id !== sourceId);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Forward message"
      description="Choose a chat to forward this message to."
      footer={
        <Button variant="secondary" type="button" onClick={onClose}>
          Cancel
        </Button>
      }
    >
      {targets.length === 0 ? (
        <p className="text-ink-muted text-sm">
          You have no other conversations to forward to.
        </p>
      ) : (
        <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
          {targets.map((conversation) => (
            <li key={conversation.id}>
              <button
                type="button"
                disabled={busy}
                onClick={() => onForward(conversation.id)}
                className="border-border-subtle hover:bg-surface-muted disabled:opacity-60 flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm"
              >
                <span className="text-ink truncate">
                  {conversationName(conversation)}
                </span>
                <span className="text-ink-muted text-xs">
                  {conversation.kind === "group" ? "Group" : "Direct"}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Dialog>
  );
}
