package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/notifications"
)

// errBoom stands in for a notification-store failure.
var errBoom = errors.New("boom")

// recordingNotifier captures the notifications a service tries to create.
type recordingNotifier struct {
	inputs []notifications.Input
	err    error
}

func (n *recordingNotifier) Notify(_ context.Context, in notifications.Input) error {
	n.inputs = append(n.inputs, in)
	return n.err
}

func TestSendRequestNotifiesRecipient(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists: true,
		from:       from,
		to:         to,
		insertReq:  Request{ID: uuid.New(), UserID: from, ContactID: to, Status: StatusPending},
		peer:       ProfileBrief{ID: to, Username: strptr("b")},
	}
	notifier := &recordingNotifier{}
	svc := NewService(fs)
	svc.SetNotifier(notifier)

	if _, _, err := svc.SendRequest(context.Background(), from, to); err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != to || got.Type != notifications.TypeContactRequest {
		t.Fatalf("unexpected notification: %+v", got)
	}
	if got.ActorID == nil || *got.ActorID != from {
		t.Fatalf("actor = %v, want %s", got.ActorID, from)
	}
}

func TestAcceptRequestNotifiesRequester(t *testing.T) {
	idReq, sender, me := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{
		getReq:    Request{ID: idReq, UserID: sender, ContactID: me, Status: StatusPending},
		acceptReq: Request{ID: idReq, UserID: sender, ContactID: me, Status: StatusAccepted},
		peer:      ProfileBrief{ID: sender, Username: strptr("s")},
	}
	notifier := &recordingNotifier{}
	svc := NewService(fs)
	svc.SetNotifier(notifier)

	if _, _, err := svc.AcceptRequest(context.Background(), idReq, me); err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != sender || got.Type != notifications.TypeContactAccepted {
		t.Fatalf("unexpected notification: %+v", got)
	}
	if got.ActorID == nil || *got.ActorID != me {
		t.Fatalf("actor = %v, want %s", got.ActorID, me)
	}
}

func TestNotificationFailureDoesNotFailRequest(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists: true,
		from:       from,
		to:         to,
		insertReq:  Request{ID: uuid.New(), UserID: from, ContactID: to, Status: StatusPending},
	}
	svc := NewService(fs)
	svc.SetNotifier(&recordingNotifier{err: errBoom})

	if _, _, err := svc.SendRequest(context.Background(), from, to); err != nil {
		t.Fatalf("SendRequest should succeed despite a notification failure: %v", err)
	}
}
