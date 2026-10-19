// Package search provides substring search over the caller's own messages,
// their conversations and groups, and public profiles, plus people discovery
// and (via the feed service) moment search. It is deliberately simple: matches
// are case-insensitive `ILIKE` substrings scoped in SQL to what the caller may
// see, with keyset pagination. There is no ranking or full-text index.
package search

import (
	"time"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// Peer is the public profile of the other side of a direct conversation.
type Peer struct {
	ID          uuid.UUID
	Username    *string
	DisplayName string
	AvatarURL   *string
}

// ConversationContext tells the client which conversation a message hit
// belongs to, so a result can be labelled and opened.
type ConversationContext struct {
	ID    uuid.UUID
	Kind  string
	Title *string
	// Peer is populated for direct conversations only.
	Peer *Peer
}

// MessageHit is one matching message plus its conversation context.
type MessageHit struct {
	Message      messaging.Message
	Conversation ConversationContext
}

// ProfileHit is one matching user profile.
type ProfileHit struct {
	ID            uuid.UUID
	Username      *string
	DisplayName   string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
	// CreatedAt orders profile results and backs their keyset cursor.
	CreatedAt time.Time
	// IsContact reports whether the viewer and this user are connected.
	IsContact bool
}

// ConversationHit is one conversation (direct or group) matched by name or, for
// direct conversations, by the peer's name. It carries enough context to render
// a search result and open the thread.
type ConversationHit struct {
	ID            uuid.UUID
	Kind          string
	Title         *string
	Peer          *Peer
	MemberCount   int64
	Role          string
	LastMessageAt *time.Time
	CreatedAt     time.Time
}

// ActivityAt is the timestamp conversation results are ordered and cursored by.
func (c ConversationHit) ActivityAt() time.Time {
	if c.LastMessageAt != nil {
		return *c.LastMessageAt
	}
	return c.CreatedAt
}

// Cursor addresses a position in message-search results, ordered by
// (created_at DESC, id DESC).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}
