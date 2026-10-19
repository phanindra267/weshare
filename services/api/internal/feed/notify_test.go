package feed

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/notifications"
)

// recordingNotifier captures the notifications the feed service creates.
type recordingNotifier struct {
	inputs []notifications.Input
}

func (n *recordingNotifier) Notify(_ context.Context, in notifications.Input) error {
	n.inputs = append(n.inputs, in)
	return nil
}

func TestLikeNotifiesMomentAuthor(t *testing.T) {
	fs := newFakeStore()
	author := uuid.New()
	viewer := uuid.New()
	id := uuid.New()
	fs.seedMoment(id, author, "a sunny day", VisibilityPublic, time.Now().UTC())

	notifier := &recordingNotifier{}
	svc := testService(fs, nil)
	svc.SetNotifier(notifier)

	if _, err := svc.Like(context.Background(), viewer, id); err != nil {
		t.Fatalf("Like: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != author || got.Type != notifications.TypeMomentLike {
		t.Fatalf("unexpected notification: %+v", got)
	}
	if got.ActorID == nil || *got.ActorID != viewer {
		t.Fatalf("actor = %v, want %s", got.ActorID, viewer)
	}
	if got.MomentID == nil || *got.MomentID != id {
		t.Fatalf("moment id = %v, want %s", got.MomentID, id)
	}
}

func TestLikeOwnMomentDoesNotNotify(t *testing.T) {
	fs := newFakeStore()
	me := uuid.New()
	id := uuid.New()
	fs.seedMoment(id, me, "mine", VisibilityPublic, time.Now().UTC())

	notifier := &recordingNotifier{}
	svc := testService(fs, nil)
	svc.SetNotifier(notifier)

	if _, err := svc.Like(context.Background(), me, id); err != nil {
		t.Fatalf("Like: %v", err)
	}
	if len(notifier.inputs) != 0 {
		t.Fatalf("liking your own moment should not notify: %+v", notifier.inputs)
	}
}

func TestAddCommentNotifiesAuthor(t *testing.T) {
	fs := newFakeStore()
	author := uuid.New()
	commenter := uuid.New()
	id := uuid.New()
	fs.seedMoment(id, author, "a sunny day", VisibilityPublic, time.Now().UTC())

	notifier := &recordingNotifier{}
	svc := testService(fs, nil)
	svc.SetNotifier(notifier)

	if _, err := svc.AddComment(context.Background(), commenter, id, "nice!", nil); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if len(notifier.inputs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.inputs))
	}
	got := notifier.inputs[0]
	if got.UserID != author || got.Type != notifications.TypeMomentComment {
		t.Fatalf("unexpected notification: %+v", got)
	}
	if got.Data["preview"] != "nice!" {
		t.Fatalf("preview = %v", got.Data["preview"])
	}
}
