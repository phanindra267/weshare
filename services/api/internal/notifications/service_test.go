package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// recordingPublisher captures every event fanned out to each user.
type recordingPublisher struct {
	events []publishedEvent
}

type publishedEvent struct {
	user  uuid.UUID
	event any
}

func (p *recordingPublisher) PublishToUser(userID uuid.UUID, event any) bool {
	p.events = append(p.events, publishedEvent{user: userID, event: event})
	return true
}

// fakeStore is an in-memory store. Notifications are held newest-first.
type fakeStore struct {
	items     []Item
	insertErr error
	deleteErr error
}

var fakeBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newFakeStore() *fakeStore { return &fakeStore{} }

func (fs *fakeStore) Insert(_ context.Context, p InsertParams) (Item, error) {
	if fs.insertErr != nil {
		return Item{}, fs.insertErr
	}
	data, _ := json.Marshal(p.Data)
	if data == nil {
		data = []byte("{}")
	}
	at := fakeBase.Add(time.Duration(len(fs.items)) * time.Second)
	item := Item{Notification: Notification{
		ID:             uuid.New(),
		UserID:         p.UserID,
		Type:           p.Type,
		ActorID:        p.ActorID,
		ConversationID: p.ConversationID,
		MessageID:      p.MessageID,
		MomentID:       p.MomentID,
		CallID:         p.CallID,
		Data:           data,
		CreatedAt:      at,
	}}
	if p.ActorID != nil {
		item.Actor = &ActorBrief{ID: *p.ActorID, DisplayName: "Actor"}
	}
	fs.items = append([]Item{item}, fs.items...)
	return item, nil
}

// isOlder reports whether item sorts strictly after the cursor position.
func olderThan(item Item, after *Cursor) bool {
	if after == nil {
		return true
	}
	if item.CreatedAt.Before(after.At) {
		return true
	}
	if item.CreatedAt.Equal(after.At) {
		return item.ID.String() < after.ID.String()
	}
	return false
}

func (fs *fakeStore) List(_ context.Context, user uuid.UUID, after *Cursor, limit int) ([]Item, error) {
	out := make([]Item, 0)
	for _, item := range fs.items {
		if item.UserID != user || !olderThan(item, after) {
			continue
		}
		out = append(out, item)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (fs *fakeStore) UnreadCount(_ context.Context, user uuid.UUID) (int64, error) {
	var n int64
	for _, item := range fs.items {
		if item.UserID == user && item.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

func (fs *fakeStore) MarkRead(_ context.Context, user uuid.UUID, ids []uuid.UUID) (int64, error) {
	wanted := map[uuid.UUID]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	now := time.Now().UTC()
	for i := range fs.items {
		item := &fs.items[i]
		if item.UserID != user || item.ReadAt != nil {
			continue
		}
		if len(ids) == 0 || wanted[item.ID] {
			item.ReadAt = &now
		}
	}
	return fs.UnreadCount(context.Background(), user)
}

func (fs *fakeStore) Delete(_ context.Context, user, id uuid.UUID) (bool, error) {
	if fs.deleteErr != nil {
		return false, fs.deleteErr
	}
	for i, item := range fs.items {
		if item.ID == id && item.UserID == user {
			fs.items = append(fs.items[:i], fs.items[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (fs *fakeStore) DeleteAll(_ context.Context, user uuid.UUID) error {
	kept := fs.items[:0]
	for _, item := range fs.items {
		if item.UserID != user {
			kept = append(kept, item)
		}
	}
	fs.items = kept
	return nil
}

func TestNotifyPersistsAndPublishes(t *testing.T) {
	st := newFakeStore()
	pub := &recordingPublisher{}
	svc := NewService(st, pub)

	recipient := uuid.New()
	actor := uuid.New()
	err := svc.Notify(context.Background(), Input{
		UserID:  recipient,
		Type:    TypeMomentLike,
		ActorID: &actor,
		Data:    map[string]any{"preview": "hello"},
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(st.items) != 1 {
		t.Fatalf("expected 1 stored notification, got %d", len(st.items))
	}
	if len(pub.events) != 1 || pub.events[0].user != recipient {
		t.Fatalf("expected one event published to recipient, got %+v", pub.events)
	}
	event, ok := pub.events[0].event.(CreatedEvent)
	if !ok {
		t.Fatalf("expected CreatedEvent, got %T", pub.events[0].event)
	}
	if event.Type != EventCreated {
		t.Errorf("event.Type = %q, want %q", event.Type, EventCreated)
	}
	if event.Notification.Type != TypeMomentLike {
		t.Errorf("notification type = %q", event.Notification.Type)
	}
	if event.Notification.Actor == nil || event.Notification.Actor.ID != actor.String() {
		t.Errorf("expected actor brief for %s", actor)
	}
	if got := event.Notification.Data["preview"]; got != "hello" {
		t.Errorf("data.preview = %v", got)
	}
}

func TestNotifySkipsSelfAction(t *testing.T) {
	st := newFakeStore()
	pub := &recordingPublisher{}
	svc := NewService(st, pub)

	me := uuid.New()
	if err := svc.Notify(context.Background(), Input{
		UserID: me, Type: TypeMomentLike, ActorID: &me,
	}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(st.items) != 0 || len(pub.events) != 0 {
		t.Fatalf("a self-notification should be dropped: items=%d events=%d",
			len(st.items), len(pub.events))
	}
}

func TestNotifyRejectsUnknownType(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	err := svc.Notify(context.Background(), Input{UserID: uuid.New(), Type: "nope"})
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "type" {
		t.Fatalf("expected a type ValidationError, got %v", err)
	}
}

func TestNotifyRejectsMissingUser(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	err := svc.Notify(context.Background(), Input{Type: TypeContactRequest})
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "user_id" {
		t.Fatalf("expected a user_id ValidationError, got %v", err)
	}
}

func TestNotifySurvivesNilPublisher(t *testing.T) {
	st := newFakeStore()
	svc := NewService(st, nil)
	if err := svc.Notify(context.Background(), Input{
		UserID: uuid.New(), Type: TypeContactRequest,
	}); err != nil {
		t.Fatalf("Notify with nil publisher: %v", err)
	}
	if len(st.items) != 1 {
		t.Fatalf("expected the notification to be persisted")
	}
}

func TestListPaginates(t *testing.T) {
	st := newFakeStore()
	svc := NewService(st, nil)
	me := uuid.New()
	for i := 0; i < 3; i++ {
		if err := svc.Notify(context.Background(), Input{
			UserID: me, Type: TypeContactRequest,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	page1, next, err := svc.List(context.Background(), me, "", 2)
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("page1: len=%d next=%q", len(page1), next)
	}
	page2, next2, err := svc.List(context.Background(), me, next, 2)
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(page2) != 1 || next2 != "" {
		t.Fatalf("page2: len=%d next=%q", len(page2), next2)
	}
	if page1[0].ID == page2[0].ID {
		t.Fatalf("pages overlap")
	}
}

func TestListRejectsBadCursor(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	_, _, err := svc.List(context.Background(), uuid.New(), "not-a-cursor", 10)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Field != "cursor" {
		t.Fatalf("expected a cursor ValidationError, got %v", err)
	}
}

func TestMarkReadAllAndRemaining(t *testing.T) {
	st := newFakeStore()
	svc := NewService(st, nil)
	me := uuid.New()
	for i := 0; i < 2; i++ {
		_ = svc.Notify(context.Background(), Input{UserID: me, Type: TypeContactRequest})
	}
	if n, _ := svc.UnreadCount(context.Background(), me); n != 2 {
		t.Fatalf("unread = %d, want 2", n)
	}
	remaining, err := svc.MarkRead(context.Background(), me, nil)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining unread = %d, want 0", remaining)
	}
}

func TestDeleteUnknownReturnsNotFound(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	err := svc.Delete(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
