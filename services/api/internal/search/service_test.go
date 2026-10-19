package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// fakeStore is a configurable in-memory search store.
type fakeStore struct {
	messages      []MessageHit
	profiles      []ProfileHit
	conversations []ConversationHit
	gotQuery      string
	gotKind       string
	gotLimit      int
	messagesFn    func(limit int) []MessageHit
}

func (fs *fakeStore) SearchMessages(_ context.Context, _ uuid.UUID, query string, after *Cursor, limit int) ([]MessageHit, error) {
	fs.gotQuery = query
	fs.gotLimit = limit
	if fs.messagesFn != nil {
		return fs.messagesFn(limit), nil
	}
	return fs.messages, nil
}

func (fs *fakeStore) SearchProfiles(_ context.Context, _ uuid.UUID, query string, _ *Cursor, limit int) ([]ProfileHit, error) {
	fs.gotQuery = query
	fs.gotLimit = limit
	return fs.profiles, nil
}

func (fs *fakeStore) DiscoverProfiles(_ context.Context, _ uuid.UUID, _ *Cursor, limit int) ([]ProfileHit, error) {
	fs.gotLimit = limit
	return fs.profiles, nil
}

func (fs *fakeStore) SearchConversations(_ context.Context, _ uuid.UUID, query, kind string, _ *Cursor, limit int) ([]ConversationHit, error) {
	fs.gotQuery = query
	fs.gotKind = kind
	fs.gotLimit = limit
	return fs.conversations, nil
}

func hitAt(id uuid.UUID, at time.Time) MessageHit {
	return MessageHit{
		Message: messaging.Message{
			ID:             id,
			ConversationID: uuid.New(),
			SenderID:       uuid.New(),
			MessageType:    "text",
			CreatedAt:      at,
			Status:         messaging.StatusReceived,
		},
		Conversation: ConversationContext{ID: uuid.New(), Kind: "direct"},
	}
}

func TestMessagesRequiresQuery(t *testing.T) {
	svc := NewService(&fakeStore{})
	_, _, err := svc.Messages(context.Background(), uuid.New(), "   ", "", 10)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "q" {
		t.Fatalf("expected q ValidationError, got %v", err)
	}
}

func TestMessagesEscapesLikeMetacharacters(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)
	if _, _, err := svc.Messages(context.Background(), uuid.New(), "50%_off", "", 10); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if fs.gotQuery != `50\%\_off` {
		t.Fatalf("query = %q, want escaped %q", fs.gotQuery, `50\%\_off`)
	}
}

func TestMessagesPaginatesAndReportsNextCursor(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fs := &fakeStore{}
	// Return limit+1 rows so the service knows a next page exists.
	fs.messagesFn = func(limit int) []MessageHit {
		out := make([]MessageHit, 0, limit)
		for i := 0; i < limit; i++ {
			out = append(out, hitAt(uuid.New(), base.Add(-time.Duration(i)*time.Minute)))
		}
		return out
	}
	svc := NewService(fs)

	hits, next, err := svc.Messages(context.Background(), uuid.New(), "hi", "", 2)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits after trimming, got %d", len(hits))
	}
	if next == "" {
		t.Fatalf("expected a next cursor")
	}
	if fs.gotLimit != 3 {
		t.Fatalf("store limit = %d, want 3 (clamped+1)", fs.gotLimit)
	}
}

func TestMessagesLastPageHasEmptyCursor(t *testing.T) {
	fs := &fakeStore{messages: []MessageHit{hitAt(uuid.New(), time.Now().UTC())}}
	svc := NewService(fs)
	hits, next, err := svc.Messages(context.Background(), uuid.New(), "hi", "", 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(hits) != 1 || next != "" {
		t.Fatalf("len=%d next=%q, want 1 and \"\"", len(hits), next)
	}
}

func TestMessagesRejectsBadCursor(t *testing.T) {
	svc := NewService(&fakeStore{})
	_, _, err := svc.Messages(context.Background(), uuid.New(), "hi", "nope", 10)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "cursor" {
		t.Fatalf("expected cursor ValidationError, got %v", err)
	}
}

func TestMessagesClampsLimit(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)
	if _, _, err := svc.Messages(context.Background(), uuid.New(), "hi", "", 9999); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if fs.gotLimit != maxLimit+1 {
		t.Fatalf("store limit = %d, want %d", fs.gotLimit, maxLimit+1)
	}
}

func TestProfilesRequiresQuery(t *testing.T) {
	svc := NewService(&fakeStore{})
	_, _, err := svc.Profiles(context.Background(), uuid.New(), "", "", 10)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "q" {
		t.Fatalf("expected q ValidationError, got %v", err)
	}
}

func TestConversationsRejectsBadKind(t *testing.T) {
	svc := NewService(&fakeStore{})
	_, _, err := svc.Conversations(context.Background(), uuid.New(), "hi", "bogus", "", 10)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "kind" {
		t.Fatalf("expected kind ValidationError, got %v", err)
	}
}

func TestConversationsEscapesQueryAndPaginates(t *testing.T) {
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	// Two rows with limit 1 => the service fetches clamped+1 and reports a page.
	fs := &fakeStore{conversations: []ConversationHit{
		{ID: uuid.New(), Kind: "group", CreatedAt: base},
		{ID: uuid.New(), Kind: "group", CreatedAt: base.Add(-time.Minute)},
	}}
	svc := NewService(fs)

	out, next, err := svc.Conversations(context.Background(), uuid.New(), "50%", "", "", 1)
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	if len(out) != 1 || next == "" {
		t.Fatalf("len=%d next=%q, want 1 and non-empty", len(out), next)
	}
	if fs.gotQuery != `50\%` {
		t.Fatalf("query = %q, want escaped", fs.gotQuery)
	}
}

func TestDiscoverPeopleDoesNotShareQuery(t *testing.T) {
	fs := &fakeStore{}
	svc := NewService(fs)
	if _, _, err := svc.DiscoverPeople(context.Background(), uuid.New(), "", 10); err != nil {
		t.Fatalf("DiscoverPeople: %v", err)
	}
	if fs.gotQuery != "" {
		t.Fatalf("discover set gotQuery = %q, want untouched", fs.gotQuery)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 3, 4, 5, 6, 7, 123456000, time.UTC)
	encoded := encodeCursor(at, id)
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if decoded == nil || !decoded.At.Equal(at) || decoded.ID != id {
		t.Fatalf("round trip = %+v, want at=%s id=%s", decoded, at, id)
	}
}
