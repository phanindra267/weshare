package search

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLimit  = 20
	maxLimit      = 50
	maxQueryRunes = 100
)

// store is the persistence surface the service depends on. *Store implements
// it; unit tests substitute a fake.
type store interface {
	SearchMessages(ctx context.Context, me uuid.UUID, query string, after *Cursor, limit int) ([]MessageHit, error)
	SearchProfiles(ctx context.Context, me uuid.UUID, query string, after *Cursor, limit int) ([]ProfileHit, error)
	DiscoverProfiles(ctx context.Context, me uuid.UUID, after *Cursor, limit int) ([]ProfileHit, error)
	SearchConversations(ctx context.Context, me uuid.UUID, query, kind string, after *Cursor, limit int) ([]ConversationHit, error)
}

// Service holds the search use cases.
type Service struct {
	store store
}

// NewService builds a search Service.
func NewService(st store) *Service { return &Service{store: st} }

// normalizeQuery trims and validates the search term.
func normalizeQuery(raw string) (string, *ValidationError) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return "", &ValidationError{Field: "q", Message: "is required"}
	}
	if len([]rune(q)) > maxQueryRunes {
		return "", &ValidationError{Field: "q", Message: "is too long"}
	}
	return q, nil
}

// escapeLike escapes the LIKE metacharacters so a user typing "%" searches for a
// literal percent sign rather than matching everything.
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > maxLimit:
		return maxLimit
	default:
		return limit
	}
}

// Messages returns one page of the caller's messages matching q, newest first,
// plus the cursor for the next page (empty when exhausted).
func (s *Service) Messages(ctx context.Context, me uuid.UUID, q, cursor string, limit int) ([]MessageHit, string, error) {
	query, verr := normalizeQuery(q)
	if verr != nil {
		return nil, "", verr
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is invalid"}
	}

	clamped := clampLimit(limit)
	// Fetch one extra row to know whether a next page exists.
	hits, err := s.store.SearchMessages(ctx, me, escapeLike(query), after, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(hits) > clamped {
		hits = hits[:clamped]
		last := hits[len(hits)-1]
		next = encodeCursor(last.Message.CreatedAt, last.Message.ID)
	}
	return hits, next, nil
}

// Profiles returns one page of public profiles matching q, newest-account
// first, plus the cursor for the next page.
func (s *Service) Profiles(ctx context.Context, me uuid.UUID, q, cursor string, limit int) ([]ProfileHit, string, error) {
	query, verr := normalizeQuery(q)
	if verr != nil {
		return nil, "", verr
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is invalid"}
	}
	clamped := clampLimit(limit)
	hits, err := s.store.SearchProfiles(ctx, me, escapeLike(query), after, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(hits) > clamped {
		hits = hits[:clamped]
		last := hits[len(hits)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return hits, next, nil
}

// DiscoverPeople returns one page of people the caller is not yet connected to,
// newest-account first, for the Discover page.
func (s *Service) DiscoverPeople(ctx context.Context, me uuid.UUID, cursor string, limit int) ([]ProfileHit, string, error) {
	after, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is invalid"}
	}
	clamped := clampLimit(limit)
	hits, err := s.store.DiscoverProfiles(ctx, me, after, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(hits) > clamped {
		hits = hits[:clamped]
		last := hits[len(hits)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	return hits, next, nil
}

// Conversations returns one page of the caller's conversations whose group
// title or direct peer name matches q. kind may be "", "direct", or "group".
func (s *Service) Conversations(ctx context.Context, me uuid.UUID, q, kind, cursor string, limit int) ([]ConversationHit, string, error) {
	query, verr := normalizeQuery(q)
	if verr != nil {
		return nil, "", verr
	}
	k := strings.TrimSpace(kind)
	if k != "" && k != "direct" && k != "group" {
		return nil, "", &ValidationError{Field: "kind", Message: "must be direct or group"}
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", &ValidationError{Field: "cursor", Message: "is invalid"}
	}
	clamped := clampLimit(limit)
	hits, err := s.store.SearchConversations(ctx, me, escapeLike(query), k, after, clamped+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(hits) > clamped {
		hits = hits[:clamped]
		last := hits[len(hits)-1]
		next = encodeCursor(last.ActivityAt(), last.ID)
	}
	return hits, next, nil
}

// encodeCursor renders "<unix-micro>:<uuid>" for a message hit.
func encodeCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UTC().UnixMicro(), 10) + ":" + id.String()
}

// decodeCursor parses the cursor produced by encodeCursor. An empty cursor is a
// valid request for the first page.
func decodeCursor(raw string) (*Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return nil, strconv.ErrSyntax
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &Cursor{At: time.UnixMicro(micros).UTC(), ID: id}, nil
}
