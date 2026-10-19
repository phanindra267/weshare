package contacts

import (
	"context"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/notifications"
)

// contactStore is the persistence surface the service depends on. The real
// implementation is *Store; tests substitute a fake to exercise the rules
// without a database.
type contactStore interface {
	UserExists(ctx context.Context, id uuid.UUID) (bool, error)
	GetRequest(ctx context.Context, id uuid.UUID) (Request, error)
	FindRequest(ctx context.Context, user, contact uuid.UUID) (Request, bool, error)
	InsertRequest(ctx context.Context, user, contact uuid.UUID) (Request, error)
	AcceptRequest(ctx context.Context, req Request) (Request, error)
	DeleteRequest(ctx context.Context, id uuid.UUID) error
	RemoveConnection(ctx context.Context, a, b uuid.UUID) (int64, error)
	Block(ctx context.Context, blocker, blocked uuid.UUID) error
	Unblock(ctx context.Context, blocker, blocked uuid.UUID) (bool, error)
	IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error)
	GetProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error)
	ListContacts(ctx context.Context, me uuid.UUID, after *uuid.UUID, limit int) ([]Contact, error)
	ListRequests(ctx context.Context, me uuid.UUID, incoming bool, after *uuid.UUID, limit int) ([]RequestWithUser, error)
}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Notifier creates a notification for a user. It is satisfied by
// *notifications.Service; a nil notifier disables notification creation.
type Notifier interface {
	Notify(ctx context.Context, in notifications.Input) error
}

// Service implements the contacts domain rules on top of a contactStore.
type Service struct {
	store    contactStore
	notifier Notifier
}

// NewService builds a contacts Service.
func NewService(store contactStore) *Service { return &Service{store: store} }

// SetNotifier wires notification creation, which is optional so REST-only
// tests and embedders can skip it.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// notify creates a notification, ignoring failures: a contact request or
// acceptance must still succeed if the notification store is unavailable.
func (s *Service) notify(ctx context.Context, in notifications.Input) {
	if s.notifier == nil {
		return
	}
	_ = s.notifier.Notify(ctx, in)
}

// clampLimit bounds a caller-supplied page size.
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

// SendRequest creates a pending request from -> to. If the reverse request is
// already pending the pair is accepted immediately (mutual add), returning a
// request with StatusAccepted. The returned ProfileBrief describes the peer.
func (s *Service) SendRequest(ctx context.Context, from, to uuid.UUID) (Request, ProfileBrief, error) {
	if from == to {
		return Request{}, ProfileBrief{}, ErrCannotAddSelf
	}

	exists, err := s.store.UserExists(ctx, to)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	if !exists {
		return Request{}, ProfileBrief{}, ErrUserNotFound
	}

	blocked, err := s.store.IsBlocked(ctx, from, to)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	if blocked {
		return Request{}, ProfileBrief{}, ErrBlocked
	}

	mine, found, err := s.store.FindRequest(ctx, from, to)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	if found {
		switch mine.Status {
		case StatusAccepted:
			return Request{}, ProfileBrief{}, ErrAlreadyFriends
		case StatusPending:
			return Request{}, ProfileBrief{}, ErrRequestAlreadySent
		}
	}

	// A pending request from the peer means a mutual add: accept it now.
	theirs, foundReverse, err := s.store.FindRequest(ctx, to, from)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	if foundReverse && theirs.Status == StatusPending {
		accepted, err := s.store.AcceptRequest(ctx, theirs)
		if err != nil {
			return Request{}, ProfileBrief{}, err
		}
		// The peer's pending request became a connection because of us.
		s.notify(ctx, notifications.Input{
			UserID:  to,
			Type:    notifications.TypeContactAccepted,
			ActorID: &from,
		})
		return accepted, s.brief(ctx, to), nil
	}

	req, err := s.store.InsertRequest(ctx, from, to)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	s.notify(ctx, notifications.Input{
		UserID:  to,
		Type:    notifications.TypeContactRequest,
		ActorID: &from,
	})
	return req, s.brief(ctx, to), nil
}

// AcceptRequest accepts the request addressed to me. The returned ProfileBrief
// describes the original sender.
func (s *Service) AcceptRequest(ctx context.Context, requestID, me uuid.UUID) (Request, ProfileBrief, error) {
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	if req.ContactID != me {
		return Request{}, ProfileBrief{}, ErrNotRecipient
	}
	if req.Status != StatusPending {
		return Request{}, ProfileBrief{}, ErrRequestNotPending
	}

	accepted, err := s.store.AcceptRequest(ctx, req)
	if err != nil {
		return Request{}, ProfileBrief{}, err
	}
	// Tell the original requester their request was accepted.
	s.notify(ctx, notifications.Input{
		UserID:  req.UserID,
		Type:    notifications.TypeContactAccepted,
		ActorID: &me,
	})
	return accepted, s.brief(ctx, req.UserID), nil
}

// DeclineRequest deletes a pending request addressed to me.
func (s *Service) DeclineRequest(ctx context.Context, requestID, me uuid.UUID) error {
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if req.ContactID != me {
		return ErrNotRecipient
	}
	if req.Status != StatusPending {
		return ErrRequestNotPending
	}
	return s.store.DeleteRequest(ctx, req.ID)
}

// RemoveContact severs the connection with other in both directions.
func (s *Service) RemoveContact(ctx context.Context, me, other uuid.UUID) error {
	if me == other {
		return ErrNotFriends
	}
	n, err := s.store.RemoveConnection(ctx, me, other)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFriends
	}
	return nil
}

// BlockUser blocks other and drops any connection. Repeated calls are
// idempotent.
func (s *Service) BlockUser(ctx context.Context, me, other uuid.UUID) error {
	if me == other {
		return ErrCannotAddSelf
	}
	exists, err := s.store.UserExists(ctx, other)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	return s.store.Block(ctx, me, other)
}

// UnblockUser removes a previous block.
func (s *Service) UnblockUser(ctx context.Context, me, other uuid.UUID) error {
	ok, err := s.store.Unblock(ctx, me, other)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotBlocked
	}
	return nil
}

// ListContacts returns one page of the caller's friendships plus the cursor
// for the next page (empty when exhausted).
func (s *Service) ListContacts(ctx context.Context, me uuid.UUID, after *uuid.UUID, limit int) ([]Contact, string, error) {
	limit = clampLimit(limit)
	items, err := s.store.ListContacts(ctx, me, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID.String()
	}
	return items, next, nil
}

// ListRequests returns one page of pending requests in the given direction
// plus the cursor for the next page (empty when exhausted).
func (s *Service) ListRequests(ctx context.Context, me uuid.UUID, incoming bool, after *uuid.UUID, limit int) ([]RequestWithUser, string, error) {
	limit = clampLimit(limit)
	items, err := s.store.ListRequests(ctx, me, incoming, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID.String()
	}
	return items, next, nil
}

// brief loads a peer profile, translating a missing user into a placeholder so
// a deleted account never fails an otherwise valid response.
func (s *Service) brief(ctx context.Context, id uuid.UUID) ProfileBrief {
	b, err := s.store.GetProfileBrief(ctx, id)
	if err != nil {
		return ProfileBrief{ID: id, DisplayName: "Unknown user"}
	}
	return b
}
