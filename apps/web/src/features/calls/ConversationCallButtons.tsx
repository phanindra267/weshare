"use client";

import { Button } from "@/components/ui/Button";
import { useCall } from "@/features/calls/CallProvider";
import type {
  Conversation,
  PeerBrief,
} from "@/features/messaging/types";

interface ConversationCallButtonsProps {
  conversation: Conversation;
}

/**
 * Audio and video call buttons for a direct conversation header. Hidden for
 * groups (group calling has no UI entry point yet) and whenever a call is
 * already in progress.
 */
export function ConversationCallButtons({
  conversation,
}: ConversationCallButtonsProps) {
  const call = useCall();

  if (conversation.kind !== "direct" || !conversation.peer) return null;
  // No CallProvider mounted (or a call is already in flight): no affordance.
  if (!call || call.state !== "idle") return null;
  const { startCall } = call;

  const peer: PeerBrief = {
    id: conversation.peer.id,
    username: conversation.peer.username,
    display_name: conversation.peer.display_name,
    avatar_url: conversation.peer.avatar_url,
    status_message: conversation.peer.status_message,
    online: conversation.peer.online,
    last_seen_at: conversation.peer.last_seen_at,
  };

  return (
    <div className="flex gap-1">
      <Button
        size="sm"
        variant="ghost"
        aria-label={`Call ${peer.display_name}`}
        onClick={() => startCall({ kind: "audio", participants: [peer] })}
      >
        Call
      </Button>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`Video call ${peer.display_name}`}
        onClick={() => startCall({ kind: "video", participants: [peer] })}
      >
        Video
      </Button>
    </div>
  );
}
