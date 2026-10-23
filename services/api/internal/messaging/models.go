package messaging

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Conversation is a chat row. `direct_key` normalizes a 1:1 pair so
// get-or-create is race-safe under a partial unique index; groups instead carry
// a `title` and optional `avatar_url`.
type Conversation struct {
	ID            uuid.UUID
	Kind          string
	CreatedBy     *uuid.UUID
	DirectKey     *string
	Title         *string
	AvatarURL     *string
	NextSequence  int64
	LastMessageAt *time.Time
	CreatedAt     time.Time
}

// ConversationSummary is one conversation as the caller sees it: the peer's
// profile (direct only), the newest message, the unread count, and — for
// groups — the caller's role and the member count.
type ConversationSummary struct {
	Conversation
	// Peer is meaningful for direct conversations. It is zero-valued for
	// groups (see Role/MemberCount), and the DTO omits it.
	Peer        ProfileBrief
	Role        string
	MemberCount int64
	LastMessage *Message
	Unread      int64
}

// MemberDetail is one conversation member joined with their public profile.
type MemberDetail struct {
	UserID        uuid.UUID
	Role          string
	DisplayName   string
	Username      *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
	JoinedAt      time.Time
}

// Message is one persisted message plus the viewer-relative delivery status.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	SenderID       uuid.UUID
	SequenceNo     int64
	ClientMsgID    *string
	MessageType    string
	Content        json.RawMessage
	CreatedAt      time.Time
	EditedAt       *time.Time
	DeletedAt      *time.Time

	// Phase 4: advanced-messaging metadata.
	ReplyToID     *uuid.UUID
	ReplyTo       *Message
	ForwardedFrom *uuid.UUID
	PinnedAt      *time.Time
	Reactions     []Reaction

	// Status is computed per requesting viewer, not stored: for the sender it
	// reflects the recipient's acknowledged read/delivered sequence; for anyone
	// else it reflects their own.
	Status string
}

// Reaction is one user's emoji reaction on a message.
type Reaction struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	Emoji     string
}

// ProfileBrief is the minimal public profile needed to render a peer.
type ProfileBrief struct {
	ID            uuid.UUID
	DisplayName   string
	Username      *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
}

// MemberReadState is a conversation member's acknowledged sequence positions.
type MemberReadState struct {
	LastReadSequence      int64
	LastDeliveredSequence int64
}
