package notifications

import (
	"context"

	"github.com/google/uuid"
)

const (
	defaultPageLimit = 30
	maxPageLimit     = 100
)

// Publisher delivers an event to every live connection of a user. It reports
// whether at least one connection received it; a notification is persisted
// regardless, so a false result just means "delivered on next list".
type Publisher interface {
	PublishToUser(userID uuid.UUID, event any) bool
}

// store is the persistence surface the service depends on. *Store implements
// it; unit tests substitute a fake.
type store interface {
	Insert(ctx context.Context, p InsertParams) (Item, error)
	List(ctx context.Context, user uuid.UUID, after *Cursor, limit int) ([]Item, error)
	UnreadCount(ctx context.Context, user uuid.UUID) (int64, error)
	MarkRead(ctx context.Context, user uuid.UUID, ids []uuid.UUID) (int64, error)
	Delete(ctx context.Context, user, id uuid.UUID) (bool, error)
	DeleteAll(ctx context.Context, user uuid.UUID) error
}

// Input describes a notification to create.
type Input struct {
	UserID         uuid.UUID
	Type           string
	ActorID        *uuid.UUID
	ConversationID *uuid.UUID
	MessageID      *uuid.UUID
	MomentID       *uuid.UUID
	CallID         *uuid.UUID
	Data           map[string]any
}

// Service holds the notification use cases.
type Service struct {
	store store
	pub   Publisher
}

// NewService builds a notifications Service. A nil publisher disables realtime
// delivery; notifications are still persisted.
func NewService(st store, pub Publisher) *Service {
	return &Service{store: st, pub: pub}
}

// Notify persists a notification and pushes it to the recipient's sockets.
// A user is never notified about their own action.
func (s *Service) Notify(ctx context.Context, in Input) error {
	if verr := validateInput(in); verr != nil {
		return verr
	}
	if in.ActorID != nil && *in.ActorID == in.UserID {
		return nil
	}

	item, err := s.store.Insert(ctx, InsertParams{
		UserID:         in.UserID,
		Type:           in.Type,
		ActorID:        in.ActorID,
		ConversationID: in.ConversationID,
		MessageID:      in.MessageID,
		MomentID:       in.MomentID,
		CallID:         in.CallID,
		Data:           in.Data,
	})
	if err != nil {
		return err
	}

	if s.pub != nil {
		s.pub.PublishToUser(in.UserID, CreatedEvent{
			Type:         EventCreated,
			Notification: toResponse(item),
		})
	}
	return nil
}

// List returns one page of the caller's notifications, newest first, plus the
// cursor for the next page (empty when exhausted).
func (s *Service) List(ctx context.Context, me uuid.UUID, cursor string, limit int) ([]Item, string, error) {
	after, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is invalid"}
	}
	clamped := clampLimit(limit)
	// Fetch one extra row to know whether a next page exists.
	items, err := s.store.List(ctx, me, after, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > clamped {
		items = items[:clamped]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return items, next, nil
}

// UnreadCount returns how many of the caller's notifications are unread.
func (s *Service) UnreadCount(ctx context.Context, me uuid.UUID) (int64, error) {
	return s.store.UnreadCount(ctx, me)
}

// MarkRead marks the given notifications read (every unread one when ids is
// empty) and returns the caller's remaining unread count.
func (s *Service) MarkRead(ctx context.Context, me uuid.UUID, ids []uuid.UUID) (int64, error) {
	return s.store.MarkRead(ctx, me, ids)
}

// Delete removes one of the caller's notifications.
func (s *Service) Delete(ctx context.Context, me, id uuid.UUID) error {
	ok, err := s.store.Delete(ctx, me, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// DeleteAll removes every notification belonging to the caller.
func (s *Service) DeleteAll(ctx context.Context, me uuid.UUID) error {
	return s.store.DeleteAll(ctx, me)
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultPageLimit
	case limit > maxPageLimit:
		return maxPageLimit
	default:
		return limit
	}
}
