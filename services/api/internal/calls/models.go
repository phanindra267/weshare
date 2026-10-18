// Package calls implements one-to-one and small-group WebRTC calling.
//
// The server never touches media: it relays SDP offers/answers and ICE
// candidates between the WebSocket connections of the users in a call, keeps an
// in-memory registry of active calls (so a user can only be on one call at a
// time), and persists a call-history summary. Only users who are accepted
// contacts with no block in either direction may call each other.
package calls

import (
	"time"

	"github.com/google/uuid"
)

// Call kind values stored in calls.kind.
const (
	KindAudio = "audio"
	KindVideo = "video"
)

// Call status values stored in calls.status.
const (
	StatusRinging  = "ringing"
	StatusActive   = "active"
	StatusEnded    = "ended"
	StatusMissed   = "missed"
	StatusRejected = "rejected"
	StatusCanceled = "canceled"
	StatusFailed   = "failed"
)

// Per-participant states stored in call_participants.state.
const (
	ParticipantInvited  = "invited"
	ParticipantJoined   = "joined"
	ParticipantRejected = "rejected"
	ParticipantLeft     = "left"
	ParticipantMissed   = "missed"
)

// Participant roles stored in call_participants.role.
const (
	RoleCaller = "caller"
	RoleCallee = "callee"
)

// ProfileBrief is the minimal public profile attached to call events and
// history rows.
type ProfileBrief struct {
	ID          uuid.UUID
	DisplayName string
	Username    *string
	AvatarURL   *string
}

// Participant is one user's membership in a call, joined with their profile.
type Participant struct {
	UserID  uuid.UUID
	Role    string
	State   string
	Profile ProfileBrief
}

// Call is a call-history record with its participants.
type Call struct {
	ID           uuid.UUID
	CallerID     uuid.UUID
	Kind         string
	IsGroup      bool
	Status       string
	StartedAt    *time.Time
	EndedAt      *time.Time
	CreatedAt    time.Time
	Participants []Participant
}

// activeCall is the in-memory state of a call that has not yet finished.
type activeCall struct {
	id        uuid.UUID
	caller    uuid.UUID
	kind      string
	group     bool
	startedAt time.Time
	// invited holds users who were rung but have not answered yet.
	invited map[uuid.UUID]struct{}
	// joined holds users who are currently in the call (the caller is present
	// from the moment they invite, since there is no separate "answer" step).
	joined map[uuid.UUID]struct{}
}
