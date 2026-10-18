package calls

import "encoding/json"

// Client-originated call event types accepted on the realtime socket. They are
// relayed by the calls service; no media is handled server-side.
const (
	ClientInvite    = "call.invite"
	ClientAccept    = "call.accept"
	ClientReject    = "call.reject"
	ClientEnd       = "call.end"
	ClientSDP       = "call.sdp"
	ClientCandidate = "call.candidate"
)

// Server-originated call event types.
const (
	EventIncoming     = "call.incoming"
	EventAccepted     = "call.accepted"
	EventRejected     = "call.rejected"
	EventBusy         = "call.busy"
	EventUnavailable  = "call.unavailable"
	EventSDP          = "call.sdp"
	EventCandidate    = "call.candidate"
	EventEnded        = "call.ended"
	EventParticipants = "call.participants"
)

// End reasons carried by EndedEvent.
const (
	EndHangup    = "hangup"
	EndCancelled = "cancelled"
	EndCompleted = "completed"
	EndFailed    = "failed"
	EndLeft      = "left"
	EndRejected  = "rejected"
)

// ClientFrame is a decoded client -> server call frame. Unknown fields are
// ignored so the protocol can grow without breaking older clients.
type ClientFrame struct {
	Type      string          `json:"type"`
	CallID    string          `json:"call_id"`
	CalleeIDs []string        `json:"callee_ids"`
	Kind      string          `json:"kind"`
	TargetID  string          `json:"target_id"`
	SDP       json.RawMessage `json:"sdp"`
	Candidate json.RawMessage `json:"candidate"`
}

// ProfileResponse is the public profile embedded in call events.
type ProfileResponse struct {
	ID          string  `json:"id"`
	Username    *string `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// IncomingEvent rings a callee. `participants` lists the users already in the
// call (for a group) so the callee knows whom to expect offers from.
type IncomingEvent struct {
	Type         string          `json:"type"`
	CallID       string          `json:"call_id"`
	From         ProfileResponse `json:"from"`
	Kind         string          `json:"kind"`
	Group        bool            `json:"group"`
	Participants []string        `json:"participants"`
}

// AcceptedEvent tells the already-joined participants that `user_id` accepted.
// A joined participant that receives it creates an SDP offer for `user_id`.
type AcceptedEvent struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	UserID string `json:"user_id"`
}

// RejectedEvent tells the remaining participants that `user_id` declined.
type RejectedEvent struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	UserID string `json:"user_id"`
}

// BusyEvent tells the caller that `user_id` is on another call.
type BusyEvent struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	UserID string `json:"user_id"`
}

// UnavailableEvent tells the caller that `user_id` is offline.
type UnavailableEvent struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	UserID string `json:"user_id"`
}

// SDPEvent relays a session description to a single peer. `from_user_id` is
// added by the server; `sdp` is passed through verbatim.
type SDPEvent struct {
	Type       string          `json:"type"`
	CallID     string          `json:"call_id"`
	FromUserID string          `json:"from_user_id"`
	SDP        json.RawMessage `json:"sdp"`
}

// CandidateEvent relays an ICE candidate to a single peer. `candidate` is
// passed through verbatim.
type CandidateEvent struct {
	Type       string          `json:"type"`
	CallID     string          `json:"call_id"`
	FromUserID string          `json:"from_user_id"`
	Candidate  json.RawMessage `json:"candidate"`
}

// EndedEvent tells participants the call (or one participant's leg of a group
// call) is over. `reason` explains why and `user_id` identifies the participant
// who left, when applicable.
type EndedEvent struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Reason string `json:"reason"`
	UserID string `json:"user_id,omitempty"`
}

// ParticipantsEvent carries the current roster of a call. Clients use it to
// render group participant lists and to drop peers that have left.
type ParticipantsEvent struct {
	Type         string            `json:"type"`
	CallID       string            `json:"call_id"`
	Participants []ProfileResponse `json:"participants"`
}

func toProfileResponse(p ProfileBrief) ProfileResponse {
	return ProfileResponse{
		ID:          p.ID.String(),
		Username:    p.Username,
		DisplayName: p.DisplayName,
		AvatarURL:   p.AvatarURL,
	}
}
