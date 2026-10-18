"use client";

import { Button } from "@/components/ui/Button";
import { useCall } from "@/features/calls/CallProvider";
import type { UserBrief } from "@/features/contacts/types";

interface ContactCallButtonsProps {
  contact: UserBrief;
}

/**
 * Audio and video call affordances for one contact row.
 *
 * Renders nothing when no `CallProvider` is mounted, so the row degrades to
 * the original contact actions rather than showing buttons that do nothing.
 * Names live in the row itself, so the buttons only carry aria-labels.
 */
export function ContactCallButtons({ contact }: ContactCallButtonsProps) {
  const call = useCall();

  if (!call || call.state !== "idle") return null;
  const { startCall } = call;

  const participants = [
    {
      id: contact.id,
      username: contact.username,
      display_name: contact.display_name,
      avatar_url: contact.avatar_url,
    },
  ];

  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`Call ${contact.display_name}`}
        onClick={() => void startCall({ kind: "audio", participants })}
      >
        Call
      </Button>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`Video call ${contact.display_name}`}
        onClick={() => void startCall({ kind: "video", participants })}
      >
        Video
      </Button>
    </>
  );
}
