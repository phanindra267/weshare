package calls

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultHistoryLimit = 30
	maxHistoryLimit     = 100
)

// Publisher fans realtime events out to connected users. The realtime hub
// implements it; unit tests can supply a stub. It reports whether the user had
// at least one live connection, which is how the service distinguishes
// "unavailable" (offline) from "ringing".
type Publisher interface {
	PublishToUser(userID uuid.UUID, event any) bool
}

// store is the persistence surface the service depends on. *Store implements
// it; unit tests use an in-memory fake.
type store interface {
	AreConnected(ctx context.Context, a, b uuid.UUID) (bool, error)
	ProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error)
	CreateCall(ctx context.Context, id, caller uuid.UUID, kind string, isGroup bool, callees []uuid.UUID) error
	MarkCallActive(ctx context.Context, id uuid.UUID) error
	FinishCall(ctx context.Context, id uuid.UUID, status string) error
	SetParticipantState(ctx context.Context, callID, userID uuid.UUID, state string) error
	ListCalls(ctx context.Context, user uuid.UUID, limit int) ([]Call, error)
}

// Service holds the calling use cases. Active calls live in memory only: the
// server relays signalling and never sees media, and if the process restarts
// any in-flight call is dropped (documented in docs/PHASE_STATUS.md).
type Service struct {
	st              store
	pub             Publisher
	maxParticipants int
	now             func() time.Time

	mu       sync.Mutex
	active   map[uuid.UUID]*activeCall
	userCall map[uuid.UUID]uuid.UUID
}

// NewService builds a calls Service. A nil publisher disables realtime delivery.
func NewService(st store, pub Publisher, maxParticipants int) *Service {
	if maxParticipants < 2 {
		maxParticipants = 2
	}
	return &Service{
		st:              st,
		pub:             pub,
		maxParticipants: maxParticipants,
		now:             time.Now,
		active:          map[uuid.UUID]*activeCall{},
		userCall:        map[uuid.UUID]uuid.UUID{},
	}
}

// MaxParticipants is the scoped participant ceiling for a single call.
func (s *Service) MaxParticipants() int { return s.maxParticipants }

// Handle dispatches one client -> server call frame.
func (s *Service) Handle(ctx context.Context, from uuid.UUID, f ClientFrame) error {
	switch f.Type {
	case ClientInvite:
		return s.Invite(ctx, from, f)
	case ClientAccept:
		return s.Accept(ctx, from, f.CallID)
	case ClientReject:
		return s.Reject(ctx, from, f.CallID)
	case ClientEnd:
		return s.End(ctx, from, f.CallID)
	case ClientSDP:
		return s.RelaySDP(ctx, from, f)
	case ClientCandidate:
		return s.RelayCandidate(ctx, from, f)
	default:
		return &ValidationError{Field: "type", Message: "unsupported call event"}
	}
}

// Invite rings one or more connected contacts on behalf of the caller. Each
// reachable callee receives a `call.incoming`; unreachable or busy callees are
// reported back to the caller.
func (s *Service) Invite(ctx context.Context, from uuid.UUID, in ClientFrame) error {
	kind, err := normalizeKind(in.Kind)
	if err != nil {
		return err
	}
	callID, err := parseCallID(in.CallID)
	if err != nil {
		return err
	}
	callees, err := parseCallees(in.CalleeIDs, from)
	if err != nil {
		return err
	}
	if len(callees) == 0 {
		return ErrSelfCall
	}
	group := len(callees) > 1
	if len(callees)+1 > s.maxParticipants {
		return ErrCallFull
	}

	// Authorization: every callee must be an accepted contact with no block.
	for _, id := range callees {
		connected, err := s.st.AreConnected(ctx, from, id)
		if err != nil {
			return err
		}
		if !connected {
			return ErrNotConnected
		}
	}

	callerProfile, err := s.st.ProfileBrief(ctx, from)
	if err != nil {
		return err
	}

	// Reserve the caller and every free callee under the lock.
	s.mu.Lock()
	if _, busy := s.userCall[from]; busy {
		s.mu.Unlock()
		return ErrBusy
	}
	call := &activeCall{
		id:      callID,
		caller:  from,
		kind:    kind,
		group:   group,
		invited: map[uuid.UUID]struct{}{},
		joined:  map[uuid.UUID]struct{}{from: {}},
	}
	invited := make([]uuid.UUID, 0, len(callees))
	busyList := make([]uuid.UUID, 0)
	for _, id := range callees {
		if _, taken := s.userCall[id]; taken {
			busyList = append(busyList, id)
			continue
		}
		s.userCall[id] = callID
		call.invited[id] = struct{}{}
		invited = append(invited, id)
	}
	if len(invited) == 0 {
		s.mu.Unlock()
		for _, id := range busyList {
			s.publish(from, BusyEvent{Type: EventBusy, CallID: callID.String(), UserID: id.String()})
		}
		// Nobody could be reached, so no call was created. Release the caller
		// explicitly: otherwise it rings forever with no way to give up.
		s.publish(from, EndedEvent{Type: EventEnded, CallID: callID.String(), Reason: EndFailed})
		return nil
	}
	s.userCall[from] = callID
	s.active[callID] = call
	s.mu.Unlock()

	if err := s.st.CreateCall(ctx, callID, from, kind, group, invited); err != nil {
		s.release(callID)
		return err
	}

	for _, id := range busyList {
		s.publish(from, BusyEvent{Type: EventBusy, CallID: callID.String(), UserID: id.String()})
	}

	// Ring each invited callee and drop the ones that are offline.
	participants := []string{from.String()}
	delivered := 0
	for _, id := range invited {
		ok := s.publish(id, IncomingEvent{
			Type:         EventIncoming,
			CallID:       callID.String(),
			From:         toProfileResponse(callerProfile),
			Kind:         kind,
			Group:        group,
			Participants: participants,
		})
		if !ok {
			s.removeFromCall(callID, id, ParticipantMissed)
			s.publish(from, UnavailableEvent{Type: EventUnavailable, CallID: callID.String(), UserID: id.String()})
			continue
		}
		delivered++
	}
	if delivered == 0 {
		s.publish(from, EndedEvent{Type: EventEnded, CallID: callID.String(), Reason: EndFailed})
		s.finish(ctx, callID, StatusFailed)
	}
	return nil
}

// Accept marks an invited callee as joined and tells the already-joined
// participants (which then create SDP offers) plus everyone's updated roster.
func (s *Service) Accept(ctx context.Context, from uuid.UUID, callIDStr string) error {
	callID, err := parseCallID(callIDStr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	call := s.active[callID]
	if call == nil {
		s.mu.Unlock()
		return ErrCallNotFound
	}
	if _, joined := call.joined[from]; joined {
		s.mu.Unlock()
		return nil // idempotent
	}
	if _, invited := call.invited[from]; !invited {
		s.mu.Unlock()
		return ErrNotParticipant
	}
	delete(call.invited, from)
	call.joined[from] = struct{}{}
	firstJoin := call.startedAt.IsZero()
	if firstJoin {
		call.startedAt = s.now().UTC()
	}
	existing := joinKeys(call.joined)
	s.mu.Unlock()

	if err := s.st.SetParticipantState(ctx, callID, from, ParticipantJoined); err != nil {
		return err
	}
	if firstJoin {
		_ = s.st.MarkCallActive(ctx, callID)
	}

	accepted := AcceptedEvent{Type: EventAccepted, CallID: callIDStr, UserID: from.String()}
	for _, id := range existing {
		if id == from {
			continue
		}
		s.publish(id, accepted)
	}
	s.broadcastParticipants(ctx, callID)
	return nil
}

// Reject declines a ringing invitation.
func (s *Service) Reject(ctx context.Context, from uuid.UUID, callIDStr string) error {
	callID, err := parseCallID(callIDStr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	call := s.active[callID]
	if call == nil {
		s.mu.Unlock()
		return ErrCallNotFound
	}
	if _, ok := call.invited[from]; !ok {
		if _, joined := call.joined[from]; !joined {
			s.mu.Unlock()
			return ErrNotParticipant
		}
	}
	delete(call.invited, from)
	delete(call.joined, from)
	if s.userCall[from] == callID {
		delete(s.userCall, from)
	}
	group := call.group
	remainingJoined := joinKeys(call.joined)
	remainingInvited := joinKeys(call.invited)
	s.mu.Unlock()

	_ = s.st.SetParticipantState(ctx, callID, from, ParticipantRejected)
	rejected := RejectedEvent{Type: EventRejected, CallID: callIDStr, UserID: from.String()}
	for _, id := range remainingJoined {
		s.publish(id, rejected)
	}

	if !group || (len(remainingJoined) == 0 && len(remainingInvited) == 0) {
		for _, id := range remainingJoined {
			s.publish(id, EndedEvent{Type: EventEnded, CallID: callIDStr, Reason: EndRejected, UserID: from.String()})
		}
		s.finish(ctx, callID, StatusRejected)
		return nil
	}
	s.broadcastParticipants(ctx, callID)
	return nil
}

// End hangs up: the caller (or either side of a direct call) tears the call
// down, while a participant leaving a group call just leaves the mesh.
func (s *Service) End(ctx context.Context, from uuid.UUID, callIDStr string) error {
	callID, err := parseCallID(callIDStr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	call := s.active[callID]
	if call == nil {
		s.mu.Unlock()
		return ErrCallNotFound
	}
	_, invited := call.invited[from]
	_, joined := call.joined[from]
	if !invited && !joined {
		s.mu.Unlock()
		return ErrNotParticipant
	}
	group := call.group
	started := !call.startedAt.IsZero()
	delete(call.invited, from)
	delete(call.joined, from)
	if s.userCall[from] == callID {
		delete(s.userCall, from)
	}
	remainingJoined := joinKeys(call.joined)
	remainingInvited := joinKeys(call.invited)
	s.mu.Unlock()

	_ = s.st.SetParticipantState(ctx, callID, from, ParticipantLeft)

	if group {
		ended := EndedEvent{Type: EventEnded, CallID: callIDStr, Reason: EndLeft, UserID: from.String()}
		for _, id := range remainingJoined {
			s.publish(id, ended)
		}
		if len(remainingJoined) == 0 && len(remainingInvited) == 0 {
			s.finish(ctx, callID, StatusEnded)
			return nil
		}
		s.broadcastParticipants(ctx, callID)
		return nil
	}

	// Direct call: tearing down ends the whole thing for both sides.
	status, reason := StatusEnded, EndHangup
	if !started {
		status, reason = StatusCanceled, EndCancelled
	}
	for _, id := range remainingJoined {
		s.publish(id, EndedEvent{Type: EventEnded, CallID: callIDStr, Reason: reason, UserID: from.String()})
	}
	for _, id := range remainingInvited {
		_ = s.st.SetParticipantState(ctx, callID, id, ParticipantMissed)
		s.publish(id, EndedEvent{Type: EventEnded, CallID: callIDStr, Reason: EndCancelled, UserID: from.String()})
	}
	s.finish(ctx, callID, status)
	return nil
}

// RelaySDP forwards an SDP offer or answer to one peer in the caller's call.
func (s *Service) RelaySDP(ctx context.Context, from uuid.UUID, in ClientFrame) error {
	if len(in.SDP) == 0 {
		return &ValidationError{Field: "sdp", Message: "sdp is required"}
	}
	return s.relay(from, in.CallID, in.TargetID, SDPEvent{
		Type:       EventSDP,
		FromUserID: from.String(),
		SDP:        in.SDP,
	})
}

// RelayCandidate forwards an ICE candidate to one peer in the caller's call.
func (s *Service) RelayCandidate(ctx context.Context, from uuid.UUID, in ClientFrame) error {
	if len(in.Candidate) == 0 {
		return &ValidationError{Field: "candidate", Message: "candidate is required"}
	}
	return s.relay(from, in.CallID, in.TargetID, CandidateEvent{
		Type:       EventCandidate,
		FromUserID: from.String(),
		Candidate:  in.Candidate,
	})
}

// relay validates that both the sender and target are joined in the call before
// delivering a signalling frame.
func (s *Service) relay(from uuid.UUID, callIDStr, targetStr string, event any) error {
	callID, err := parseCallID(callIDStr)
	if err != nil {
		return err
	}
	target, err := uuid.Parse(strings.TrimSpace(targetStr))
	if err != nil {
		return &ValidationError{Field: "target_id", Message: "target_id must be a UUID"}
	}

	s.mu.Lock()
	call := s.active[callID]
	if call == nil {
		s.mu.Unlock()
		return ErrCallNotFound
	}
	if _, ok := call.joined[from]; !ok {
		s.mu.Unlock()
		return ErrNotParticipant
	}
	if _, ok := call.joined[target]; !ok {
		s.mu.Unlock()
		return ErrNotParticipant
	}
	s.mu.Unlock()

	// Stamp the call id onto the event's wire shape.
	event = withCallID(event, callIDStr)
	s.publish(target, event)
	return nil
}

// HandleDisconnect tears down a user's active call when their last socket
// closes. It is a no-op when the user has no call in progress.
func (s *Service) HandleDisconnect(ctx context.Context, userID uuid.UUID) {
	s.mu.Lock()
	callID, ok := s.userCall[userID]
	s.mu.Unlock()
	if !ok {
		return
	}
	// End is safe to call here; an already-finished call returns an error that
	// is meaningless on the disconnect path.
	_ = s.End(ctx, userID, callID.String())
}

// History returns a user's recent calls, newest first.
func (s *Service) History(ctx context.Context, user uuid.UUID, limit int) ([]Call, error) {
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	return s.st.ListCalls(ctx, user, limit)
}

// --- internals ---------------------------------------------------------------

// publish delivers an event through the hub, tolerating a nil publisher.
func (s *Service) publish(userID uuid.UUID, event any) bool {
	if s.pub == nil {
		return false
	}
	return s.pub.PublishToUser(userID, event)
}

// broadcastParticipants sends the current joined roster to everyone in a call.
func (s *Service) broadcastParticipants(ctx context.Context, callID uuid.UUID) {
	s.mu.Lock()
	call := s.active[callID]
	if call == nil {
		s.mu.Unlock()
		return
	}
	ids := joinKeys(call.joined)
	s.mu.Unlock()

	profiles := make([]ProfileResponse, 0, len(ids))
	for _, id := range ids {
		p, err := s.st.ProfileBrief(ctx, id)
		if err != nil {
			continue
		}
		profiles = append(profiles, toProfileResponse(p))
	}
	event := ParticipantsEvent{Type: EventParticipants, CallID: callID.String(), Participants: profiles}
	for _, id := range ids {
		s.publish(id, event)
	}
}

// removeFromCall drops one participant from the in-memory call and records the
// given terminal state, without touching the other participants.
func (s *Service) removeFromCall(callID, userID uuid.UUID, state string) {
	s.mu.Lock()
	if call := s.active[callID]; call != nil {
		delete(call.invited, userID)
		delete(call.joined, userID)
	}
	if s.userCall[userID] == callID {
		delete(s.userCall, userID)
	}
	s.mu.Unlock()
	_ = s.st.SetParticipantState(context.Background(), callID, userID, state)
}

// finish sets a terminal status and forgets the in-memory call.
func (s *Service) finish(ctx context.Context, callID uuid.UUID, status string) {
	_ = s.st.FinishCall(ctx, callID, status)
	s.release(callID)
}

// release drops a call and all of its user -> call mappings.
func (s *Service) release(callID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := s.active[callID]
	if call == nil {
		return
	}
	for id := range call.joined {
		if s.userCall[id] == callID {
			delete(s.userCall, id)
		}
	}
	for id := range call.invited {
		if s.userCall[id] == callID {
			delete(s.userCall, id)
		}
	}
	delete(s.active, callID)
}

// joinKeys returns the sorted keys of a set, so event ordering is deterministic.
func joinKeys(set map[uuid.UUID]struct{}) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// withCallID fills in the call id on a relayed event (SDPEvent/CandidateEvent
// are built before the call id is parsed in relay).
func withCallID(event any, callID string) any {
	switch e := event.(type) {
	case SDPEvent:
		e.CallID = callID
		return e
	case CandidateEvent:
		e.CallID = callID
		return e
	default:
		return event
	}
}

func normalizeKind(kind string) (string, error) {
	switch strings.TrimSpace(kind) {
	case "", KindAudio:
		return KindAudio, nil
	case KindVideo:
		return KindVideo, nil
	default:
		return "", &ValidationError{Field: "kind", Message: "must be audio or video"}
	}
}

func parseCallID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, &ValidationError{Field: "call_id", Message: "call_id must be a UUID"}
	}
	return id, nil
}

func parseCallees(raw []string, self uuid.UUID) ([]uuid.UUID, error) {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		id, err := uuid.Parse(trimmed)
		if err != nil {
			return nil, &ValidationError{Field: "callee_ids", Message: "contains an invalid user id"}
		}
		if id == self {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}
