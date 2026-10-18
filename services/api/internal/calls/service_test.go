package calls

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// --- fakes -------------------------------------------------------------------

type publishedEvent struct {
	user  uuid.UUID
	event any
}

// recordingPublisher captures every event fanned out to each user and reports
// whether a user is "connected" (has a live socket).
type recordingPublisher struct {
	connected map[uuid.UUID]bool
	events    []publishedEvent
}

func newPublisher(connected ...uuid.UUID) *recordingPublisher {
	p := &recordingPublisher{connected: map[uuid.UUID]bool{}}
	for _, id := range connected {
		p.connected[id] = true
	}
	return p
}

func (p *recordingPublisher) PublishToUser(userID uuid.UUID, event any) bool {
	p.events = append(p.events, publishedEvent{user: userID, event: event})
	return p.connected[userID]
}

// eventsFor returns the events delivered to one user, in order.
func (p *recordingPublisher) eventsFor(user uuid.UUID) []any {
	out := []any{}
	for _, e := range p.events {
		if e.user == user {
			out = append(out, e.event)
		}
	}
	return out
}

// hasEvent reports whether user received an event of the given concrete type.
func (p *recordingPublisher) hasEvent(user uuid.UUID, want any) bool {
	for _, e := range p.eventsFor(user) {
		switch want.(type) {
		case IncomingEvent:
			if _, ok := e.(IncomingEvent); ok {
				return true
			}
		case AcceptedEvent:
			if _, ok := e.(AcceptedEvent); ok {
				return true
			}
		case RejectedEvent:
			if _, ok := e.(RejectedEvent); ok {
				return true
			}
		case BusyEvent:
			if _, ok := e.(BusyEvent); ok {
				return true
			}
		case UnavailableEvent:
			if _, ok := e.(UnavailableEvent); ok {
				return true
			}
		case SDPEvent:
			if _, ok := e.(SDPEvent); ok {
				return true
			}
		case CandidateEvent:
			if _, ok := e.(CandidateEvent); ok {
				return true
			}
		case ParticipantsEvent:
			if _, ok := e.(ParticipantsEvent); ok {
				return true
			}
		case EndedEvent:
			if _, ok := e.(EndedEvent); ok {
				return true
			}
		}
	}
	return false
}

func (p *recordingPublisher) endedReason(user uuid.UUID) (string, bool) {
	for _, e := range p.eventsFor(user) {
		if ev, ok := e.(EndedEvent); ok {
			return ev.Reason, true
		}
	}
	return "", false
}

// fakeStore is an in-memory implementation of the store interface.
type fakeStore struct {
	connected map[string]bool
	profiles  map[uuid.UUID]ProfileBrief
	status    map[uuid.UUID]string
	states    map[uuid.UUID]map[uuid.UUID]string
	callees   map[uuid.UUID][]uuid.UUID
	history   []Call
	active    map[uuid.UUID]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		connected: map[string]bool{},
		profiles:  map[uuid.UUID]ProfileBrief{},
		status:    map[uuid.UUID]string{},
		states:    map[uuid.UUID]map[uuid.UUID]string{},
		callees:   map[uuid.UUID][]uuid.UUID{},
		active:    map[uuid.UUID]bool{},
	}
}

func connKey(a, b uuid.UUID) string {
	if a.String() < b.String() {
		return a.String() + "|" + b.String()
	}
	return b.String() + "|" + a.String()
}

func (f *fakeStore) setProfile(p ProfileBrief) { f.profiles[p.ID] = p }

func (f *fakeStore) AreConnected(_ context.Context, a, b uuid.UUID) (bool, error) {
	if v, ok := f.connected[connKey(a, b)]; ok {
		return v, nil
	}
	return true, nil
}

func (f *fakeStore) ProfileBrief(_ context.Context, id uuid.UUID) (ProfileBrief, error) {
	p, ok := f.profiles[id]
	if !ok {
		return ProfileBrief{}, errors.New("profile not found")
	}
	return p, nil
}

func (f *fakeStore) CreateCall(_ context.Context, id, caller uuid.UUID, kind string, isGroup bool, callees []uuid.UUID) error {
	f.status[id] = StatusRinging
	f.callees[id] = callees
	f.states[id] = map[uuid.UUID]string{caller: ParticipantJoined}
	for _, c := range callees {
		f.states[id][c] = ParticipantInvited
	}
	return nil
}

func (f *fakeStore) MarkCallActive(_ context.Context, id uuid.UUID) error {
	f.active[id] = true
	if f.status[id] == StatusRinging {
		f.status[id] = StatusActive
	}
	return nil
}

func (f *fakeStore) FinishCall(_ context.Context, id uuid.UUID, status string) error {
	if f.status[id] == StatusRinging || f.status[id] == StatusActive || f.status[id] == "" {
		f.status[id] = status
	}
	return nil
}

func (f *fakeStore) SetParticipantState(_ context.Context, callID, userID uuid.UUID, state string) error {
	if f.states[callID] == nil {
		f.states[callID] = map[uuid.UUID]string{}
	}
	f.states[callID][userID] = state
	return nil
}

func (f *fakeStore) ListCalls(_ context.Context, _ uuid.UUID, _ int) ([]Call, error) {
	return f.history, nil
}

// --- helpers -----------------------------------------------------------------

func newUser(t *testing.T, st *fakeStore, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	uname := name
	st.setProfile(ProfileBrief{ID: id, DisplayName: name, Username: &uname})
	return id
}

func callID() string { return uuid.New().String() }

// --- tests -------------------------------------------------------------------

func TestInviteRequiresAcceptedContact(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	st.connected[connKey(a, b)] = false
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)

	err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{b.String()}, Kind: KindAudio})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
	if len(st.status) != 0 {
		t.Fatalf("no call should be persisted, got %v", st.status)
	}
}

func TestInviteSelfOnlyIsRejected(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	svc := NewService(st, newPublisher(a), 4)
	err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{a.String()}})
	if !errors.Is(err, ErrSelfCall) {
		t.Fatalf("want ErrSelfCall, got %v", err)
	}
}

func TestInviteDeliversIncomingAndAccept(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "alice")
	b := newUser(t, st, "bob")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String()}, Kind: KindVideo}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	// Bob is rung with alice's profile and the initial roster.
	var incoming *IncomingEvent
	for _, e := range pub.eventsFor(b) {
		if ev, ok := e.(IncomingEvent); ok {
			incoming = &ev
		}
	}
	if incoming == nil {
		t.Fatal("bob did not receive call.incoming")
	}
	if incoming.Kind != KindVideo || incoming.Group {
		t.Fatalf("unexpected incoming: %+v", incoming)
	}
	if incoming.From.DisplayName != "alice" {
		t.Fatalf("incoming.from.display_name = %q", incoming.From.DisplayName)
	}
	if len(incoming.Participants) != 1 || incoming.Participants[0] != a.String() {
		t.Fatalf("unexpected roster: %v", incoming.Participants)
	}
	if st.status[uuid.MustParse(id)] != StatusRinging {
		t.Fatalf("call status = %q, want ringing", st.status[uuid.MustParse(id)])
	}

	// Bob accepts: alice learns to offer, both get the roster.
	if err := svc.Accept(context.Background(), b, id); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if !pub.hasEvent(a, AcceptedEvent{}) {
		t.Fatal("alice did not receive call.accepted")
	}
	if !pub.hasEvent(a, ParticipantsEvent{}) || !pub.hasEvent(b, ParticipantsEvent{}) {
		t.Fatal("participants roster not broadcast")
	}
	if st.status[uuid.MustParse(id)] != StatusActive {
		t.Fatalf("call status = %q, want active", st.status[uuid.MustParse(id)])
	}
}

func TestInviteBusyCalleeNotifiesCaller(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	pub := newPublisher(a, b, c)
	svc := NewService(st, pub, 4)

	// b is already in a call with c.
	if err := svc.Invite(context.Background(), b, ClientFrame{CallID: callID(), CalleeIDs: []string{c.String()}}); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	// a tries to call b.
	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{b.String()}}); err != nil {
		t.Fatalf("second invite: %v", err)
	}
	if !pub.hasEvent(a, BusyEvent{}) {
		t.Fatal("alice did not receive call.busy")
	}
	// No call was created, so the caller must also be released rather than
	// ringing until it gives up.
	if reason, ok := pub.endedReason(a); !ok || reason != EndFailed {
		t.Fatalf("ended reason = %q,%v want failed", reason, ok)
	}
}

func TestInvitePartiallyBusyKeepsCallAlive(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	d := newUser(t, st, "d")
	pub := newPublisher(a, b, c, d)
	svc := NewService(st, pub, 4)

	// b and c are tied up on their own call, so only d is reachable.
	if err := svc.Invite(context.Background(), b, ClientFrame{CallID: callID(), CalleeIDs: []string{c.String()}}); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	id := callID()
	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String(), d.String()}, Kind: KindAudio}); err != nil {
		t.Fatalf("second invite: %v", err)
	}
	if !pub.hasEvent(a, BusyEvent{}) {
		t.Fatal("alice did not receive call.busy for b")
	}
	if !pub.hasEvent(d, IncomingEvent{}) {
		t.Fatal("d did not receive call.incoming")
	}
	// One reachable callee means the call stands: ending it would hang up on d.
	if reason, ok := pub.endedReason(a); ok {
		t.Fatalf("alice received call.ended (%q) despite a reachable callee", reason)
	}
	if st.status[uuid.MustParse(id)] == StatusFailed {
		t.Fatal("call was marked failed despite a reachable callee")
	}
}

func TestInviteBusyCallerReturnsError(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{b.String()}}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{b.String()}})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("want ErrBusy, got %v", err)
	}
}

func TestInviteOfflineCalleeFailsCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a) // b has no live socket
	svc := NewService(st, pub, 4)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String()}}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if !pub.hasEvent(a, UnavailableEvent{}) {
		t.Fatal("alice did not receive call.unavailable")
	}
	if reason, ok := pub.endedReason(a); !ok || reason != EndFailed {
		t.Fatalf("ended reason = %q,%v want failed", reason, ok)
	}
	if st.status[uuid.MustParse(id)] != StatusFailed {
		t.Fatalf("call status = %q, want failed", st.status[uuid.MustParse(id)])
	}
}

func TestRejectNotifiesCaller(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String()}}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := svc.Reject(context.Background(), b, id); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if !pub.hasEvent(a, RejectedEvent{}) {
		t.Fatal("alice did not receive call.rejected")
	}
	if reason, ok := pub.endedReason(a); !ok || reason != EndRejected {
		t.Fatalf("ended reason = %q,%v want rejected", reason, ok)
	}
	if st.status[uuid.MustParse(id)] != StatusRejected {
		t.Fatalf("call status = %q, want rejected", st.status[uuid.MustParse(id)])
	}
	if st.states[uuid.MustParse(id)][b] != ParticipantRejected {
		t.Fatalf("bob state = %q, want rejected", st.states[uuid.MustParse(id)][b])
	}
}

func TestCallerCancelsRingingCallMarksMissed(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String()}}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := svc.End(context.Background(), a, id); err != nil {
		t.Fatalf("end: %v", err)
	}
	if reason, ok := pub.endedReason(b); !ok || reason != EndCancelled {
		t.Fatalf("bob ended reason = %q,%v want cancelled", reason, ok)
	}
	if st.status[uuid.MustParse(id)] != StatusCanceled {
		t.Fatalf("call status = %q, want canceled", st.status[uuid.MustParse(id)])
	}
	if st.states[uuid.MustParse(id)][b] != ParticipantMissed {
		t.Fatalf("bob state = %q, want missed", st.states[uuid.MustParse(id)][b])
	}
}

func TestHangupActiveDirectCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	mustInvite(t, svc, a, id, b)
	mustAccept(t, svc, b, id)

	if err := svc.End(context.Background(), a, id); err != nil {
		t.Fatalf("end: %v", err)
	}
	if reason, ok := pub.endedReason(b); !ok || reason != EndHangup {
		t.Fatalf("bob ended reason = %q,%v want hangup", reason, ok)
	}
	if st.status[uuid.MustParse(id)] != StatusEnded {
		t.Fatalf("call status = %q, want ended", st.status[uuid.MustParse(id)])
	}
}

func TestGroupInviteAcceptLeave(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	pub := newPublisher(a, b, c)
	svc := NewService(st, pub, 4)
	id := callID()

	if err := svc.Invite(context.Background(), a, ClientFrame{CallID: id, CalleeIDs: []string{b.String(), c.String()}}); err != nil {
		t.Fatalf("invite: %v", err)
	}
	for _, callee := range []uuid.UUID{b, c} {
		var incoming *IncomingEvent
		for _, e := range pub.eventsFor(callee) {
			if ev, ok := e.(IncomingEvent); ok {
				incoming = &ev
			}
		}
		if incoming == nil || !incoming.Group {
			t.Fatalf("callee %s did not get a group invite", callee)
		}
	}

	mustAccept(t, svc, b, id)
	mustAccept(t, svc, c, id)

	// b leaves; a and c are told b left, and the call stays up.
	if err := svc.End(context.Background(), b, id); err != nil {
		t.Fatalf("end: %v", err)
	}
	leftSeen := false
	for _, e := range pub.eventsFor(a) {
		if ev, ok := e.(EndedEvent); ok && ev.UserID == b.String() && ev.Reason == EndLeft {
			leftSeen = true
		}
	}
	if !leftSeen {
		t.Fatal("alice was not told bob left the group call")
	}
	if st.status[uuid.MustParse(id)] == StatusEnded {
		t.Fatal("group call should stay active after one participant leaves")
	}
}

func TestRelaySDPRequiresMembershipAndStampsSender(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	pub := newPublisher(a, b, c)
	svc := NewService(st, pub, 4)
	id := callID()

	mustInvite(t, svc, a, id, b)
	mustAccept(t, svc, b, id)

	sdp := json.RawMessage(`{"type":"offer","sdp":"v=0"}`)
	if err := svc.RelaySDP(context.Background(), a, ClientFrame{CallID: id, TargetID: b.String(), SDP: sdp}); err != nil {
		t.Fatalf("relay sdp: %v", err)
	}
	var got *SDPEvent
	for _, e := range pub.eventsFor(b) {
		if ev, ok := e.(SDPEvent); ok {
			got = &ev
		}
	}
	if got == nil {
		t.Fatal("bob did not receive call.sdp")
	}
	if got.FromUserID != a.String() {
		t.Fatalf("from_user_id = %q, want %q", got.FromUserID, a)
	}
	if got.CallID != id {
		t.Fatalf("call_id = %q, want %q", got.CallID, id)
	}
	if string(got.SDP) != string(sdp) {
		t.Fatalf("sdp = %s, want %s", got.SDP, sdp)
	}

	// A non-member cannot be a relay target.
	if err := svc.RelaySDP(context.Background(), a, ClientFrame{CallID: id, TargetID: c.String(), SDP: sdp}); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("want ErrNotParticipant, got %v", err)
	}
}

func TestRelayCandidate(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	mustInvite(t, svc, a, id, b)
	mustAccept(t, svc, b, id)

	candidate := json.RawMessage(`{"candidate":"candidate:1"}`)
	if err := svc.RelayCandidate(context.Background(), b, ClientFrame{CallID: id, TargetID: a.String(), Candidate: candidate}); err != nil {
		t.Fatalf("relay candidate: %v", err)
	}
	if !pub.hasEvent(a, CandidateEvent{}) {
		t.Fatal("alice did not receive call.candidate")
	}
}

func TestMaxParticipants(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	pub := newPublisher(a, b, c)
	svc := NewService(st, pub, 2)

	err := svc.Invite(context.Background(), a, ClientFrame{CallID: callID(), CalleeIDs: []string{b.String(), c.String()}})
	if !errors.Is(err, ErrCallFull) {
		t.Fatalf("want ErrCallFull, got %v", err)
	}
}

func TestHandleDisconnectEndsCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	pub := newPublisher(a, b)
	svc := NewService(st, pub, 4)
	id := callID()

	mustInvite(t, svc, a, id, b)
	mustAccept(t, svc, b, id)

	svc.HandleDisconnect(context.Background(), b)
	if reason, ok := pub.endedReason(a); !ok || reason != EndHangup {
		t.Fatalf("alice ended reason = %q,%v want hangup", reason, ok)
	}
	for _, e := range pub.eventsFor(b) {
		if ev, ok := e.(EndedEvent); ok && ev.Reason != EndHangup {
			t.Fatalf("unexpected ended reason for bob: %q", ev.Reason)
		}
	}
}

func TestAcceptUnknownOrForeignCall(t *testing.T) {
	st := newFakeStore()
	a := newUser(t, st, "a")
	b := newUser(t, st, "b")
	c := newUser(t, st, "c")
	svc := NewService(st, newPublisher(a, b, c), 4)

	if err := svc.Accept(context.Background(), b, callID()); !errors.Is(err, ErrCallNotFound) {
		t.Fatalf("unknown call: want ErrCallNotFound, got %v", err)
	}

	id := callID()
	mustInvite(t, svc, a, id, b)
	if err := svc.Accept(context.Background(), c, id); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("foreign call: want ErrNotParticipant, got %v", err)
	}
}

func TestNormalizeKind(t *testing.T) {
	cases := map[string]string{
		"":      KindAudio,
		"audio": KindAudio,
		"video": KindVideo,
	}
	for in, want := range cases {
		got, err := normalizeKind(in)
		if err != nil || got != want {
			t.Fatalf("normalizeKind(%q) = %q,%v want %q", in, got, err, want)
		}
	}
	if _, err := normalizeKind("hologram"); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestParseCalleesDedupesAndDropsSelf(t *testing.T) {
	self := uuid.New()
	other := uuid.New()
	got, err := parseCallees([]string{self.String(), other.String(), other.String(), " "}, self)
	if err != nil {
		t.Fatalf("parseCallees: %v", err)
	}
	if len(got) != 1 || got[0] != other {
		t.Fatalf("got %v, want [%s]", got, other)
	}
	if _, err := parseCallees([]string{"not-a-uuid"}, self); err == nil {
		t.Fatal("expected error for invalid uuid")
	}
}

// --- small helpers -----------------------------------------------------------

func mustInvite(t *testing.T, svc *Service, from uuid.UUID, id string, callees ...uuid.UUID) {
	t.Helper()
	ids := make([]string, 0, len(callees))
	for _, c := range callees {
		ids = append(ids, c.String())
	}
	if err := svc.Invite(context.Background(), from, ClientFrame{CallID: id, CalleeIDs: ids}); err != nil {
		t.Fatalf("invite: %v", err)
	}
}

func mustAccept(t *testing.T, svc *Service, user uuid.UUID, id string) {
	t.Helper()
	if err := svc.Accept(context.Background(), user, id); err != nil {
		t.Fatalf("accept: %v", err)
	}
}
