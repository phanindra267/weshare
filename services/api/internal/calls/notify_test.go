package calls

import (
	"context"
	"testing"

	"github.com/phanindra267/weshare/services/api/internal/notifications"
)

// recordingNotifier captures the notifications the calls service creates.
type recordingNotifier struct {
	inputs []notifications.Input
}

func (n *recordingNotifier) Notify(_ context.Context, in notifications.Input) error {
	n.inputs = append(n.inputs, in)
	return nil
}

func TestInviteOfflineCalleeLeavesMissedCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "alice")
	b := newUser(t, st, "bob")
	// Only alice has a live socket; bob is offline.
	pub := newPublisher(a)
	notifier := &recordingNotifier{}
	svc := NewService(st, pub, 4)
	svc.SetNotifier(notifier)

	if err := svc.Invite(context.Background(), a, ClientFrame{
		CallID: callID(), CalleeIDs: []string{b.String()}, Kind: KindVideo,
	}); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 missed-call notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != b || got.Type != notifications.TypeMissedCall {
		t.Fatalf("unexpected notification: %+v", got)
	}
	if got.ActorID == nil || *got.ActorID != a {
		t.Fatalf("actor = %v, want %s", got.ActorID, a)
	}
	if got.CallID == nil {
		t.Fatalf("missed call should reference the call id")
	}
	if got.Data["kind"] != KindVideo {
		t.Fatalf("kind = %v, want video", got.Data["kind"])
	}
}

func TestCallerHangupBeforeAnswerLeavesMissedCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "alice")
	b := newUser(t, st, "bob")
	pub := newPublisher(a, b)
	notifier := &recordingNotifier{}
	svc := NewService(st, pub, 4)
	svc.SetNotifier(notifier)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{
		CallID: id, CalleeIDs: []string{b.String()}, Kind: KindAudio,
	}); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	// Bob never answered; alice hangs up.
	if err := svc.End(context.Background(), a, id); err != nil {
		t.Fatalf("End: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 missed-call notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != b || got.Type != notifications.TypeMissedCall {
		t.Fatalf("unexpected notification: %+v", got)
	}
}

func TestAnsweredCallLeavesNoMissedCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "alice")
	b := newUser(t, st, "bob")
	pub := newPublisher(a, b)
	notifier := &recordingNotifier{}
	svc := NewService(st, pub, 4)
	svc.SetNotifier(notifier)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{
		CallID: id, CalleeIDs: []string{b.String()}, Kind: KindAudio,
	}); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if err := svc.Accept(context.Background(), b, id); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := svc.End(context.Background(), a, id); err != nil {
		t.Fatalf("End: %v", err)
	}
	if len(notifier.inputs) != 0 {
		t.Fatalf("an answered-then-ended call should leave no missed call: %+v", notifier.inputs)
	}
}
