// Package notifications persists per-user notifications and pushes a
// `notification.created` frame over the realtime socket. Feature packages
// (contacts, feed, calls) create notifications through the Service, which owns
// validation, fan-out, and the read/delete lifecycle.
package notifications

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Notification types. These are the wire discriminator persisted in the `type`
// column and mirrored in packages/contracts/websocket-events.md.
const (
	TypeContactRequest  = "contact_request"
	TypeContactAccepted = "contact_accepted"
	TypeMomentLike      = "moment_like"
	TypeMomentComment   = "moment_comment"
	TypeMissedCall      = "missed_call"
)

// Notification is one persisted notification row.
type Notification struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Type           string
	ActorID        *uuid.UUID
	ConversationID *uuid.UUID
	MessageID      *uuid.UUID
	MomentID       *uuid.UUID
	CallID         *uuid.UUID
	Data           json.RawMessage
	ReadAt         *time.Time
	CreatedAt      time.Time
}

// ActorBrief is the public profile of whoever triggered the notification.
type ActorBrief struct {
	ID          uuid.UUID
	DisplayName string
	Username    *string
	AvatarURL   *string
}

// Item is a notification joined with its actor's public profile.
type Item struct {
	Notification
	Actor *ActorBrief
}

// Cursor addresses a position in a user's notification history. Ordering is
// (created_at DESC, id DESC), so both parts are needed for a stable page.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}
