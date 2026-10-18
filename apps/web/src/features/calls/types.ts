// Call feature types. Mirror the Go backend in `services/api/internal/calls` and
// the realtime protocol in `packages/contracts/websocket-events.md`.

export type CallKind = "audio" | "video";

export interface PeerBrief {
  id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
}

export interface CallParticipant {
  user_id: string;
  username: string | null;
  display_name: string;
  avatar_url: string | null;
  role: "caller" | "callee";
  state: "invited" | "joined" | "rejected" | "left" | "missed";
}

export interface CallIncomingEvent {
  type: "call.incoming";
  call_id: string;
  from: PeerBrief;
  kind: CallKind;
  group: boolean;
  participants: string[];
}

export interface CallAcceptedEvent {
  type: "call.accepted";
  call_id: string;
  user_id: string;
}

export interface CallRejectedEvent {
  type: "call.rejected";
  call_id: string;
  user_id: string;
}

export interface CallBusyEvent {
  type: "call.busy";
  call_id: string;
  user_id: string;
}

export interface CallUnavailableEvent {
  type: "call.unavailable";
  call_id: string;
  user_id: string;
}

export interface CallSDPEvent {
  type: "call.sdp";
  call_id: string;
  from_user_id: string;
  sdp: unknown;
}

export interface CallCandidateEvent {
  type: "call.candidate";
  call_id: string;
  from_user_id: string;
  candidate: unknown;
}

export interface CallEndedEvent {
  type: "call.ended";
  call_id: string;
  reason:
    | "hangup"
    | "cancelled"
    | "completed"
    | "failed"
    | "left"
    | "rejected";
  user_id?: string;
}

export interface CallParticipantsEvent {
  type: "call.participants";
  call_id: string;
  participants: PeerBrief[];
}

export type CallServerEvent =
  | CallIncomingEvent
  | CallAcceptedEvent
  | CallRejectedEvent
  | CallBusyEvent
  | CallUnavailableEvent
  | CallSDPEvent
  | CallCandidateEvent
  | CallEndedEvent
  | CallParticipantsEvent;

export interface CallHistoryItem {
  id: string;
  kind: CallKind;
  group: boolean;
  status: string;
  direction: "outgoing" | "incoming";
  outcome: string;
  started_at: string | null;
  ended_at: string | null;
  created_at: string;
  duration_seconds: number;
  participants: CallParticipant[];
}

export interface CallIceConfig {
  ice_servers: Array<{
    urls: string | string[];
    username?: string;
    credential?: string;
  }>;
  max_participants: number;
}
