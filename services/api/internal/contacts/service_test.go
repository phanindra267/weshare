package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func strptr(s string) *string { return &s }

func TestSendRequest_CannotAddSelf(t *testing.T) {
	s := NewService(&fakeStore{})
	id := uuid.New()
	_, _, err := s.SendRequest(context.Background(), id, id)
	if !errors.Is(err, ErrCannotAddSelf) {
		t.Fatalf("want ErrCannotAddSelf, got %v", err)
	}
}

func TestSendRequest_UserNotFound(t *testing.T) {
	fs := &fakeStore{userExists: false}
	_, _, err := NewService(fs).SendRequest(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestSendRequest_Blocked(t *testing.T) {
	fs := &fakeStore{userExists: true, isBlocked: true}
	_, _, err := NewService(fs).SendRequest(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}

func TestSendRequest_AlreadyFriends(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists: true,
		from:       from,
		to:         to,
		mineFound:  true,
		mine:       Request{Status: StatusAccepted},
	}
	_, _, err := NewService(fs).SendRequest(context.Background(), from, to)
	if !errors.Is(err, ErrAlreadyFriends) {
		t.Fatalf("want ErrAlreadyFriends, got %v", err)
	}
}

func TestSendRequest_AlreadySent(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists: true,
		from:       from,
		to:         to,
		mineFound:  true,
		mine:       Request{Status: StatusPending},
	}
	_, _, err := NewService(fs).SendRequest(context.Background(), from, to)
	if !errors.Is(err, ErrRequestAlreadySent) {
		t.Fatalf("want ErrRequestAlreadySent, got %v", err)
	}
}

func TestSendRequest_MutualAccepts(t *testing.T) {
	idA, idB := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists:  true,
		from:        idA,
		to:          idB,
		mineFound:   false,
		theirsFound: true,
		theirs:      Request{ID: uuid.New(), UserID: idB, ContactID: idA, Status: StatusPending},
		acceptReq:   Request{ID: uuid.New(), UserID: idB, ContactID: idA, Status: StatusAccepted},
		peer:        ProfileBrief{ID: idB, Username: strptr("b")},
	}
	req, peer, err := NewService(fs).SendRequest(context.Background(), idA, idB)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if req.Status != StatusAccepted {
		t.Fatalf("want accepted, got %s", req.Status)
	}
	if peer.Username == nil || *peer.Username != "b" {
		t.Fatalf("peer: %+v", peer)
	}
}

func TestSendRequest_InsertsNew(t *testing.T) {
	idA, idB := uuid.New(), uuid.New()
	fs := &fakeStore{
		userExists: true,
		from:       idA,
		to:         idB,
		insertReq:  Request{ID: uuid.New(), UserID: idA, ContactID: idB, Status: StatusPending},
		peer:       ProfileBrief{ID: idB, Username: strptr("b")},
	}
	req, peer, err := NewService(fs).SendRequest(context.Background(), idA, idB)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if req.Status != StatusPending {
		t.Fatalf("want pending, got %s", req.Status)
	}
	if peer.Username == nil || *peer.Username != "b" {
		t.Fatalf("peer: %+v", peer)
	}
}

func TestAcceptRequest_RequiresRecipient(t *testing.T) {
	idReq, other, me := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{getReq: Request{ID: idReq, UserID: other, ContactID: other, Status: StatusPending}}
	_, _, err := NewService(fs).AcceptRequest(context.Background(), idReq, me)
	if !errors.Is(err, ErrNotRecipient) {
		t.Fatalf("want ErrNotRecipient, got %v", err)
	}
}

func TestAcceptRequest_NotPending(t *testing.T) {
	idReq, sender, me := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{getReq: Request{ID: idReq, UserID: sender, ContactID: me, Status: StatusAccepted}}
	_, _, err := NewService(fs).AcceptRequest(context.Background(), idReq, me)
	if !errors.Is(err, ErrRequestNotPending) {
		t.Fatalf("want ErrRequestNotPending, got %v", err)
	}
}

func TestAcceptRequest_Ok(t *testing.T) {
	idReq, sender, me := uuid.New(), uuid.New(), uuid.New()
	fs := &fakeStore{
		getReq:    Request{ID: idReq, UserID: sender, ContactID: me, Status: StatusPending},
		acceptReq: Request{ID: idReq, UserID: sender, ContactID: me, Status: StatusAccepted},
		peer:      ProfileBrief{ID: sender, Username: strptr("s")},
	}
	req, peer, err := NewService(fs).AcceptRequest(context.Background(), idReq, me)
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	if req.Status != StatusAccepted {
		t.Fatalf("want accepted, got %s", req.Status)
	}
	if peer.Username == nil || *peer.Username != "s" {
		t.Fatalf("peer: %+v", peer)
	}
}

func TestUnblockUser_NotBlocked(t *testing.T) {
	fs := &fakeStore{unblockOK: false}
	err := NewService(fs).UnblockUser(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotBlocked) {
		t.Fatalf("want ErrNotBlocked, got %v", err)
	}
}

func TestRemoveContact_NotFriends(t *testing.T) {
	fs := &fakeStore{removeCount: 0}
	err := NewService(fs).RemoveContact(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFriends) {
		t.Fatalf("want ErrNotFriends, got %v", err)
	}
}

func TestClampLimit(t *testing.T) {
	if got := clampLimit(0); got != defaultListLimit {
		t.Fatalf("clampLimit(0) = %d, want %d", got, defaultListLimit)
	}
	if got := clampLimit(maxListLimit + 100); got != maxListLimit {
		t.Fatalf("clampLimit(over) = %d, want %d", got, maxListLimit)
	}
	if got := clampLimit(10); got != 10 {
		t.Fatalf("clampLimit(10) = %d, want 10", got)
	}
}

func TestSendRequestInputValidate(t *testing.T) {
	var in sendRequestInput
	if errs := in.Validate(); errs == nil || errs["user_id"] == "" {
		t.Fatalf("empty user_id should be rejected")
	}
	in.UserID = "not-a-uuid"
	if errs := in.Validate(); errs == nil || errs["user_id"] == "" {
		t.Fatalf("malformed user_id should be rejected")
	}
	in.UserID = uuid.New().String()
	if errs := in.Validate(); len(errs) != 0 {
		t.Fatalf("valid user_id rejected: %v", errs)
	}
}

type fakeStore struct {
	userExists    bool
	userExistsErr error
	peer          ProfileBrief
	peerErr       error
	getReq        Request
	getErr        error
	from          uuid.UUID
	to            uuid.UUID
	mine          Request
	mineFound     bool
	theirs        Request
	theirsFound   bool
	insertReq     Request
	insertErr     error
	acceptReq     Request
	acceptErr     error
	deleteErr     error
	removeCount   int64
	removeErr     error
	blockErr      error
	unblockOK     bool
	unblockErr    error
	isBlocked     bool
	isBlockedErr  error
	contacts      []Contact
	contactsErr   error
	reqs          []RequestWithUser
	requestsErr   error
}

func (f *fakeStore) UserExists(context.Context, uuid.UUID) (bool, error) {
	return f.userExists, f.userExistsErr
}

func (f *fakeStore) GetProfileBrief(context.Context, uuid.UUID) (ProfileBrief, error) {
	return f.peer, f.peerErr
}

func (f *fakeStore) GetRequest(context.Context, uuid.UUID) (Request, error) {
	return f.getReq, f.getErr
}

func (f *fakeStore) FindRequest(_ context.Context, user, contact uuid.UUID) (Request, bool, error) {
	switch {
	case user == f.from && contact == f.to:
		return f.mine, f.mineFound, nil
	case user == f.to && contact == f.from:
		return f.theirs, f.theirsFound, nil
	default:
		return Request{}, false, nil
	}
}

func (f *fakeStore) InsertRequest(context.Context, uuid.UUID, uuid.UUID) (Request, error) {
	return f.insertReq, f.insertErr
}

func (f *fakeStore) AcceptRequest(context.Context, Request) (Request, error) {
	return f.acceptReq, f.acceptErr
}

func (f *fakeStore) DeleteRequest(context.Context, uuid.UUID) error { return f.deleteErr }

func (f *fakeStore) RemoveConnection(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return f.removeCount, f.removeErr
}

func (f *fakeStore) Block(context.Context, uuid.UUID, uuid.UUID) error { return f.blockErr }

func (f *fakeStore) Unblock(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.unblockOK, f.unblockErr
}

func (f *fakeStore) IsBlocked(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.isBlocked, f.isBlockedErr
}

func (f *fakeStore) ListContacts(context.Context, uuid.UUID, *uuid.UUID, int) ([]Contact, error) {
	return f.contacts, f.contactsErr
}

func (f *fakeStore) ListRequests(context.Context, uuid.UUID, bool, *uuid.UUID, int) ([]RequestWithUser, error) {
	return f.reqs, f.requestsErr
}
