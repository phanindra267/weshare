package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// recordingPublisher captures every event fanned out to each user and reports
// whether a user is "connected".
type recordingPublisher struct {
	connected map[uuid.UUID]bool
	events    []publishedEvent
}

type publishedEvent struct {
	user  uuid.UUID
	event any
}

func newRecordingPublisher(connected ...uuid.UUID) *recordingPublisher {
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

// fakeStore is a configurable in-memory store for service tests.
type fakeStore struct {
	members    map[uuid.UUID]map[uuid.UUID]bool
	states     map[uuid.UUID]map[uuid.UUID]MemberReadState
	nextSeq    map[uuid.UUID]int64
	summaries  []ConversationSummary
	summary    ConversationSummary
	conv       Conversation
	userExists bool
	canMessage bool
	inserted   Message
	insertDup  bool
	lastInsert InsertMessageParams
	messages   []Message
	hasMore    bool
	statuses   map[uuid.UUID]string
	advanceErr error
	unread     int64

	// Group operations (Phase 4).
	groupConv     Conversation
	updatedGroup  Conversation
	createdGroup  Conversation
	roles         map[uuid.UUID]map[uuid.UUID]string
	memberDetails []MemberDetail
	profiles      map[uuid.UUID]ProfileBrief
	addedCount    int64
	removed       bool
	roleSet       bool
	leaveDeleted  bool
	leaveNewOwner *uuid.UUID

	// Advanced messaging (Phase 4).
	message        Message
	messageErr     error
	messagesByID   map[uuid.UUID]Message
	edited         Message
	reactions      map[uuid.UUID][]Reaction
	deletedIDs     []uuid.UUID
	pinnedAt       *time.Time
	pinnedMessages []Message
	searchResults  []Message
	searchHasMore  bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		members:      map[uuid.UUID]map[uuid.UUID]bool{},
		states:       map[uuid.UUID]map[uuid.UUID]MemberReadState{},
		nextSeq:      map[uuid.UUID]int64{},
		roles:        map[uuid.UUID]map[uuid.UUID]string{},
		profiles:     map[uuid.UUID]ProfileBrief{},
		messagesByID: map[uuid.UUID]Message{},
		reactions:    map[uuid.UUID][]Reaction{},
	}
}

func (f *fakeStore) ListConversations(context.Context, uuid.UUID, *ConversationCursor, int) ([]ConversationSummary, error) {
	return f.summaries, nil
}

func (f *fakeStore) GetConversationSummary(context.Context, uuid.UUID, uuid.UUID) (ConversationSummary, error) {
	return f.summary, nil
}

func (f *fakeStore) IsMember(_ context.Context, conv, user uuid.UUID) (bool, error) {
	return f.members[conv][user], nil
}

func (f *fakeStore) MemberIDs(_ context.Context, conv uuid.UUID) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	for id := range f.members[conv] {
		out = append(out, id)
	}
	return out, nil
}

func (f *fakeStore) MemberState(_ context.Context, conv, user uuid.UUID) (MemberReadState, error) {
	return f.states[conv][user], nil
}

func (f *fakeStore) FindOrCreateDirect(_ context.Context, _, _ uuid.UUID) (Conversation, bool, error) {
	return f.conv, false, nil
}

func (f *fakeStore) UserExists(context.Context, uuid.UUID) (bool, error) {
	return f.userExists, nil
}

func (f *fakeStore) CanMessage(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.canMessage, nil
}

func (f *fakeStore) InsertMessage(_ context.Context, p InsertMessageParams) (Message, bool, error) {
	f.lastInsert = p
	// Mirror the real store: echo the persisted reply/forward links onto the
	// returned row so callers see exactly what was written.
	if f.inserted.ForwardedFrom == nil {
		f.inserted.ForwardedFrom = p.ForwardedFrom
	}
	if f.inserted.ReplyToID == nil {
		f.inserted.ReplyToID = p.ReplyToID
	}
	if f.inserted.ConversationID == uuid.Nil {
		f.inserted.ConversationID = p.ConversationID
	}
	if f.inserted.SenderID == uuid.Nil {
		f.inserted.SenderID = p.SenderID
	}
	return f.inserted, f.insertDup, nil
}

func (f *fakeStore) ListMessages(context.Context, uuid.UUID, uuid.UUID, MessageQuery) ([]Message, bool, error) {
	return f.messages, f.hasMore, nil
}

func (f *fakeStore) StatusFor(_ context.Context, _, viewer, _ uuid.UUID, _ int64) (string, error) {
	if s, ok := f.statuses[viewer]; ok {
		return s, nil
	}
	return StatusSent, nil
}

func (f *fakeStore) AdvanceRead(_ context.Context, conv, user uuid.UUID, upTo int64) (int64, error) {
	if _, ok := f.members[conv][user]; !ok {
		return 0, ErrNotMember
	}
	st := f.states[conv][user]
	if upTo > st.LastReadSequence {
		st.LastReadSequence = upTo
	}
	f.states[conv][user] = st
	return st.LastReadSequence, nil
}

func (f *fakeStore) AdvanceDelivered(_ context.Context, conv, user uuid.UUID, upTo int64) (int64, error) {
	if f.advanceErr != nil {
		return 0, f.advanceErr
	}
	st := f.states[conv][user]
	if upTo <= st.LastDeliveredSequence {
		return 0, ErrConversationNotFound
	}
	st.LastDeliveredSequence = upTo
	f.states[conv][user] = st
	return st.LastDeliveredSequence, nil
}

func (f *fakeStore) AdvanceDeliveredForUser(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, f.advanceErr
}

func (f *fakeStore) UnreadCount(context.Context, uuid.UUID, uuid.UUID, int64) (int64, error) {
	return f.unread, nil
}

func (f *fakeStore) PeekSequence(_ context.Context, conv uuid.UUID) (int64, error) {
	return f.nextSeq[conv], nil
}

func (f *fakeStore) Conversation(_ context.Context, id uuid.UUID) (Conversation, error) {
	if f.groupConv.ID != id {
		return Conversation{}, ErrConversationNotFound
	}
	return f.groupConv, nil
}

func (f *fakeStore) ProfileBrief(_ context.Context, id uuid.UUID) (ProfileBrief, error) {
	return f.profiles[id], nil
}

func (f *fakeStore) MemberRole(_ context.Context, conv, user uuid.UUID) (string, error) {
	if role, ok := f.roles[conv][user]; ok {
		return role, nil
	}
	return "", ErrNotMember
}

func (f *fakeStore) CountMembers(_ context.Context, conv uuid.UUID) (int64, error) {
	return int64(len(f.members[conv])), nil
}

func (f *fakeStore) MemberDetails(context.Context, uuid.UUID) ([]MemberDetail, error) {
	return f.memberDetails, nil
}

func (f *fakeStore) CreateGroup(_ context.Context, owner uuid.UUID, title string, memberIDs []uuid.UUID) (Conversation, error) {
	conv := f.createdGroup
	if conv.ID == uuid.Nil {
		conv = Conversation{ID: uuid.New(), Kind: ConversationKindGroup}
	}
	conv.Kind = ConversationKindGroup
	conv.Title = &title
	if f.members[conv.ID] == nil {
		f.members[conv.ID] = map[uuid.UUID]bool{}
	}
	if f.roles[conv.ID] == nil {
		f.roles[conv.ID] = map[uuid.UUID]string{}
	}
	f.members[conv.ID][owner] = true
	f.roles[conv.ID][owner] = RoleOwner
	for _, id := range memberIDs {
		f.members[conv.ID][id] = true
		if f.roles[conv.ID][id] == "" {
			f.roles[conv.ID][id] = RoleMember
		}
	}
	f.createdGroup = conv
	return conv, nil
}

func (f *fakeStore) UpdateGroupMeta(_ context.Context, id uuid.UUID, title, avatarURL *string) (Conversation, error) {
	conv := f.updatedGroup
	if conv.ID == uuid.Nil {
		conv = f.groupConv
	}
	if title != nil {
		conv.Title = title
	}
	if avatarURL != nil {
		conv.AvatarURL = avatarURL
	}
	return conv, nil
}

func (f *fakeStore) AddMembers(_ context.Context, id uuid.UUID, userIDs []uuid.UUID) (int64, error) {
	for _, user := range userIDs {
		if f.members[id] == nil {
			f.members[id] = map[uuid.UUID]bool{}
		}
		f.members[id][user] = true
	}
	if f.addedCount > 0 {
		return f.addedCount, nil
	}
	return int64(len(userIDs)), nil
}

func (f *fakeStore) RemoveMember(_ context.Context, conv, user uuid.UUID) (bool, error) {
	if f.removed {
		delete(f.members[conv], user)
		delete(f.roles[conv], user)
	}
	return f.removed, nil
}

func (f *fakeStore) SetMemberRole(_ context.Context, conv, user uuid.UUID, role string) (bool, error) {
	if f.roleSet {
		if f.roles[conv] == nil {
			f.roles[conv] = map[uuid.UUID]string{}
		}
		f.roles[conv][user] = role
	}
	return f.roleSet, nil
}

func (f *fakeStore) LeaveGroup(_ context.Context, conv, user uuid.UUID) (bool, *uuid.UUID, error) {
	if !f.leaveDeleted {
		delete(f.members[conv], user)
		delete(f.roles[conv], user)
	}
	return f.leaveDeleted, f.leaveNewOwner, nil
}

// --- advanced messaging (Phase 4) -------------------------------------------

func (f *fakeStore) MessageFor(_ context.Context, _, _ uuid.UUID) (Message, error) {
	if f.messageErr != nil {
		return Message{}, f.messageErr
	}
	return f.message, nil
}

func (f *fakeStore) MessagesByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]Message, error) {
	out := map[uuid.UUID]Message{}
	for _, id := range ids {
		if m, ok := f.messagesByID[id]; ok {
			out[id] = m
		}
	}
	return out, nil
}

func (f *fakeStore) EditMessage(_ context.Context, _ uuid.UUID, content json.RawMessage, editedAt time.Time) (Message, error) {
	f.edited.Content = content
	f.edited.EditedAt = &editedAt
	return f.edited, nil
}

func (f *fakeStore) SoftDeleteMessage(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeStore) AddReaction(_ context.Context, messageID, userID uuid.UUID, emoji string) (bool, error) {
	f.reactions[messageID] = append(f.reactions[messageID],
		Reaction{MessageID: messageID, UserID: userID, Emoji: emoji})
	return true, nil
}

func (f *fakeStore) RemoveReaction(_ context.Context, messageID, userID uuid.UUID, emoji string) (bool, error) {
	list := f.reactions[messageID]
	kept := make([]Reaction, 0, len(list))
	removed := false
	for _, r := range list {
		if r.UserID == userID && r.Emoji == emoji {
			removed = true
			continue
		}
		kept = append(kept, r)
	}
	f.reactions[messageID] = kept
	return removed, nil
}

func (f *fakeStore) ReactionsFor(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]Reaction, error) {
	out := map[uuid.UUID][]Reaction{}
	for _, id := range ids {
		if rs, ok := f.reactions[id]; ok {
			out[id] = rs
		}
	}
	return out, nil
}

func (f *fakeStore) PinMessage(_ context.Context, _ uuid.UUID, pinnedAt *time.Time) error {
	f.pinnedAt = pinnedAt
	return nil
}

func (f *fakeStore) PinnedMessages(context.Context, uuid.UUID) ([]Message, error) {
	return f.pinnedMessages, nil
}

func (f *fakeStore) SearchMessages(context.Context, uuid.UUID, uuid.UUID, string, *int64, int) ([]Message, bool, error) {
	return f.searchResults, f.searchHasMore, nil
}

// --- OpenConversation -------------------------------------------------------

func TestOpenConversation_Self(t *testing.T) {
	id := uuid.New()
	_, err := NewService(newFakeStore(), nil).OpenConversation(context.Background(), id, id)
	if !errors.Is(err, ErrSelfConversation) {
		t.Fatalf("want ErrSelfConversation, got %v", err)
	}
}

func TestOpenConversation_PeerNotFound(t *testing.T) {
	fs := newFakeStore()
	fs.userExists = false
	_, err := NewService(fs, nil).OpenConversation(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("want ErrPeerNotFound, got %v", err)
	}
}

func TestOpenConversation_NotConnected(t *testing.T) {
	fs := newFakeStore()
	fs.userExists = true
	fs.canMessage = false
	_, err := NewService(fs, nil).OpenConversation(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}

func TestOpenConversation_OK(t *testing.T) {
	me, peer := uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.userExists = true
	fs.canMessage = true
	fs.summary = ConversationSummary{Conversation: Conversation{ID: uuid.New()}, Peer: ProfileBrief{ID: peer}}
	got, err := NewService(fs, nil).OpenConversation(context.Background(), me, peer)
	if err != nil {
		t.Fatalf("OpenConversation: %v", err)
	}
	if got.Peer.ID != peer {
		t.Fatalf("peer id = %s, want %s", got.Peer.ID, peer)
	}
}

// --- SendMessage ------------------------------------------------------------

func TestSendMessage_ValidationRejectsEmptyText(t *testing.T) {
	fs := newFakeStore()
	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"   "}`)}
	_, _, err := NewService(fs, nil).SendMessage(context.Background(), uuid.New(), uuid.New(), &in)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if invalid.Fields["content"] == "" {
		t.Fatalf("expected content field error, got %v", invalid.Fields)
	}
}

func TestSendMessage_NotMember(t *testing.T) {
	fs := newFakeStore()
	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`)}
	_, _, err := NewService(fs, nil).SendMessage(context.Background(), uuid.New(), uuid.New(), &in)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("want ErrConversationNotFound, got %v", err)
	}
}

func TestSendMessage_FanOutAndDelivered(t *testing.T) {
	me, peer, conv := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true, peer: true}
	fs.states[conv] = map[uuid.UUID]MemberReadState{me: {}, peer: {}}
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, SenderID: me, SequenceNo: 1, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`)}
	fs.statuses = map[uuid.UUID]string{me: StatusSent}

	pub := newRecordingPublisher(peer) // peer is online; sender is not
	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`)}
	msg, dup, err := NewService(fs, pub).SendMessage(context.Background(), me, conv, &in)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if dup {
		t.Fatal("unexpected duplicate")
	}
	if msg.Status != StatusSent {
		t.Fatalf("sender status = %s, want sent", msg.Status)
	}

	// Both members receive message.created.
	for _, u := range []uuid.UUID{me, peer} {
		found := false
		for _, e := range pub.events {
			if e.user == u {
				if _, ok := e.event.(MessageCreatedEvent); ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("user %s did not receive message.created", u)
		}
	}

	// The online peer caused a delivered receipt back to the sender.
	found := false
	for _, e := range pub.events {
		if e.user == me {
			if r, ok := e.event.(MessageReceiptEvent); ok && r.Status == StatusDelivered && r.UserID == peer.String() {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("sender did not receive a delivered receipt")
	}
}

func TestSendMessage_DuplicateDoesNotFanOut(t *testing.T) {
	me, peer, conv := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true, peer: true}
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, SenderID: me, SequenceNo: 1}
	fs.insertDup = true
	fs.statuses = map[uuid.UUID]string{me: StatusSent}

	pub := newRecordingPublisher(peer)
	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`)}
	_, dup, err := NewService(fs, pub).SendMessage(context.Background(), me, conv, &in)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !dup {
		t.Fatal("expected duplicate=true")
	}
	if len(pub.events) != 0 {
		t.Fatalf("duplicate send should not fan out, got %d events", len(pub.events))
	}
}

// --- MarkRead ---------------------------------------------------------------

func TestMarkRead_ClampsToEnd(t *testing.T) {
	me, peer, conv := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true, peer: true}
	fs.states[conv] = map[uuid.UUID]MemberReadState{me: {}, peer: {}}
	fs.nextSeq[conv] = 3 // only 3 messages exist
	fs.unread = 0

	pub := newRecordingPublisher(peer)
	unread, err := NewService(fs, pub).MarkRead(context.Background(), me, conv, 99)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if unread != 0 {
		t.Fatalf("unread = %d, want 0", unread)
	}
	// Position clamped to 3, not 99.
	if got := fs.states[conv][me].LastReadSequence; got != 3 {
		t.Fatalf("last read = %d, want 3", got)
	}
	// Peer is notified of the read.
	found := false
	for _, e := range pub.events {
		if e.user == peer {
			if r, ok := e.event.(MessageReceiptEvent); ok && r.Status == StatusRead {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("peer did not receive a read receipt")
	}
}

func TestMarkRead_NotMember(t *testing.T) {
	fs := newFakeStore()
	_, err := NewService(fs, nil).MarkRead(context.Background(), uuid.New(), uuid.New(), 1)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("want ErrConversationNotFound, got %v", err)
	}
}

func TestMarkRead_NegativeSequence(t *testing.T) {
	fs := newFakeStore()
	conv, me := uuid.New(), uuid.New()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	_, err := NewService(fs, nil).MarkRead(context.Background(), me, conv, -1)
	if !errors.Is(err, ErrInvalidSequence) {
		t.Fatalf("want ErrInvalidSequence, got %v", err)
	}
}

// --- ListMessages -----------------------------------------------------------

func TestListMessages_NotMember(t *testing.T) {
	fs := newFakeStore()
	_, _, err := NewService(fs, nil).ListMessages(context.Background(), uuid.New(), uuid.New(), MessageQuery{})
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("want ErrConversationNotFound, got %v", err)
	}
}

func TestListMessages_RejectsBeforeAndAfter(t *testing.T) {
	fs := newFakeStore()
	conv, me := uuid.New(), uuid.New()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	before, after := int64(5), int64(1)
	_, _, err := NewService(fs, nil).ListMessages(context.Background(), me, conv, MessageQuery{Before: &before, After: &after})
	if !errors.Is(err, ErrInvalidSequence) {
		t.Fatalf("want ErrInvalidSequence, got %v", err)
	}
}

func TestListMessages_CursorPointsAtOldest(t *testing.T) {
	fs := newFakeStore()
	conv, me := uuid.New(), uuid.New()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	fs.messages = []Message{{SequenceNo: 10}, {SequenceNo: 11}, {SequenceNo: 12}}
	fs.hasMore = true

	_, next, err := NewService(fs, nil).ListMessages(context.Background(), me, conv, MessageQuery{Limit: 3})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if next != "10" {
		t.Fatalf("next = %q, want 10", next)
	}
}

func TestListMessages_NoCursorWhenExhausted(t *testing.T) {
	fs := newFakeStore()
	conv, me := uuid.New(), uuid.New()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	fs.messages = []Message{{SequenceNo: 10}}
	fs.hasMore = false

	_, next, err := NewService(fs, nil).ListMessages(context.Background(), me, conv, MessageQuery{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if next != "" {
		t.Fatalf("next = %q, want empty", next)
	}
}

// --- validation -------------------------------------------------------------

func TestSendInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		in      SendInput
		wantKey string
	}{
		{"missing client id", SendInput{MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`)}, "client_msg_id"},
		{"empty text", SendInput{ClientMsgID: "c", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":""}`)}, "content"},
		{"unknown key", SendInput{ClientMsgID: "c", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi","bogus":1}`)}, "content"},
		{"system type", SendInput{ClientMsgID: "c", MessageType: MessageTypeSystem, Content: json.RawMessage(`{"text":"hi"}`)}, "message_type"},
		{"bad type", SendInput{ClientMsgID: "c", MessageType: "unknown", Content: json.RawMessage(`{"text":"hi"}`)}, "message_type"},
		{"image needs url", SendInput{ClientMsgID: "c", MessageType: MessageTypeImage, Content: json.RawMessage(`{}`)}, "content"},
		{"video needs url", SendInput{ClientMsgID: "c", MessageType: MessageTypeVideo, Content: json.RawMessage(`{}`)}, "content"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := tc.in.Validate()
			if fields[tc.wantKey] == "" {
				t.Fatalf("want error on %q, got %v", tc.wantKey, fields)
			}
		})
	}

	ok := SendInput{ClientMsgID: "c", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"  hello  "}`)}
	if fields := ok.Validate(); len(fields) != 0 {
		t.Fatalf("valid input rejected: %v", fields)
	}
	// Text is normalized (trimmed) before persistence.
	var body textContent
	if err := json.Unmarshal(ok.Content, &body); err != nil || body.Text != "hello" {
		t.Fatalf("normalized content = %s (err %v)", ok.Content, err)
	}
	// Default type is applied when omitted.
	defaulted := SendInput{ClientMsgID: "c", Content: json.RawMessage(`{"text":"hi"}`)}
	if fields := defaulted.Validate(); len(fields) != 0 {
		t.Fatalf("default type rejected: %v", fields)
	}
	if defaulted.MessageType != MessageTypeText {
		t.Fatalf("default type = %q, want text", defaulted.MessageType)
	}
}

func TestClampLimitMessaging(t *testing.T) {
	if got := clampLimit(0); got != defaultPageLimit {
		t.Fatalf("clampLimit(0) = %d, want %d", got, defaultPageLimit)
	}
	if got := clampLimit(maxPageLimit + 100); got != maxPageLimit {
		t.Fatalf("clampLimit(over) = %d, want %d", got, maxPageLimit)
	}
	if got := clampLimit(10); got != 10 {
		t.Fatalf("clampLimit(10) = %d, want 10", got)
	}
}

func TestConversationCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	c := ConversationCursor{At: time.Now().UTC().Truncate(time.Microsecond), ID: id}
	parsed, err := ParseConversationCursor(c.String())
	if err != nil {
		t.Fatalf("ParseConversationCursor: %v", err)
	}
	if parsed.ID != id || !parsed.At.Equal(c.At) {
		t.Fatalf("round trip = %+v, want %+v", parsed, c)
	}
	if _, err := ParseConversationCursor("nonsense"); err == nil {
		t.Fatal("expected error for malformed cursor")
	}
}

// --- groups -----------------------------------------------------------------

// groupStore builds a fake with a group conversation and the given role holders.
func groupStore(conv, owner uuid.UUID, withRole map[uuid.UUID]string) *fakeStore {
	fs := newFakeStore()
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.members[conv] = map[uuid.UUID]bool{owner: true}
	fs.roles[conv] = map[uuid.UUID]string{owner: RoleOwner}
	for id, role := range withRole {
		fs.members[conv][id] = true
		fs.roles[conv][id] = role
	}
	return fs
}

func hasEvent[T any](pub *recordingPublisher, user uuid.UUID) bool {
	for _, e := range pub.events {
		if e.user != user {
			continue
		}
		if _, ok := e.event.(T); ok {
			return true
		}
	}
	return false
}

func TestCreateGroup_OK(t *testing.T) {
	me, a, b, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.userExists = true
	fs.canMessage = true
	fs.createdGroup = Conversation{ID: conv}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.summary = ConversationSummary{Conversation: Conversation{ID: conv, Kind: ConversationKindGroup}, Role: RoleOwner, MemberCount: 3}
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}
	fs.profiles[me] = ProfileBrief{ID: me, DisplayName: "Me"}

	pub := newRecordingPublisher()
	got, err := NewService(fs, pub).CreateGroup(context.Background(), me, "Team", []uuid.UUID{a, b})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if got.ID != conv || got.Role != RoleOwner || got.MemberCount != 3 {
		t.Fatalf("summary = %+v", got)
	}
	if fs.roles[conv][me] != RoleOwner {
		t.Fatalf("owner role = %q", fs.roles[conv][me])
	}
	if !fs.members[conv][a] || !fs.members[conv][b] {
		t.Fatalf("members = %v", fs.members[conv])
	}
	// Every member (including the creator) learns the conversation and the
	// newly added members.
	for _, u := range []uuid.UUID{me, a, b} {
		if !hasEvent[MessageCreatedEvent](pub, u) {
			t.Fatalf("user %s missing message.created", u)
		}
		if !hasEvent[ConversationUpdatedEvent](pub, u) {
			t.Fatalf("user %s missing conversation.updated", u)
		}
	}
	if !hasEvent[MemberChangedEvent](pub, a) {
		t.Fatal("invitee a missing member.changed")
	}
}

func TestCreateGroup_DedupesSelfAndDuplicates(t *testing.T) {
	me, a, conv := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.userExists = true
	fs.canMessage = true
	fs.createdGroup = Conversation{ID: conv}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.summary = ConversationSummary{Conversation: Conversation{ID: conv}}

	if _, err := NewService(fs, nil).CreateGroup(context.Background(), me, "Team", []uuid.UUID{a, a, me, uuid.Nil}); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if len(fs.members[conv]) != 2 {
		t.Fatalf("members = %v, want owner + one invitee", fs.members[conv])
	}
}

func TestCreateGroup_UnknownUser(t *testing.T) {
	fs := newFakeStore()
	fs.userExists = false
	_, err := NewService(fs, nil).CreateGroup(context.Background(), uuid.New(), "Team", []uuid.UUID{uuid.New()})
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("want ErrPeerNotFound, got %v", err)
	}
}

func TestCreateGroup_NotConnected(t *testing.T) {
	fs := newFakeStore()
	fs.userExists = true
	fs.canMessage = false
	_, err := NewService(fs, nil).CreateGroup(context.Background(), uuid.New(), "Team", []uuid.UUID{uuid.New()})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
}

func TestLoadGroup_DirectIsNotGroup(t *testing.T) {
	me, conv := uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindDirect}
	_, err := NewService(fs, nil).GetMembers(context.Background(), me, conv)
	if !errors.Is(err, ErrNotGroup) {
		t.Fatalf("want ErrNotGroup, got %v", err)
	}
}

func TestLoadGroup_NonMemberIsNotFound(t *testing.T) {
	owner, conv := uuid.New(), uuid.New()
	fs := groupStore(conv, owner, nil)
	_, err := NewService(fs, nil).GetMembers(context.Background(), uuid.New(), conv)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("want ErrConversationNotFound, got %v", err)
	}
}

func TestGetMembers_OK(t *testing.T) {
	owner, member, conv := uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember})
	fs.memberDetails = []MemberDetail{{UserID: owner, Role: RoleOwner}, {UserID: member, Role: RoleMember}}

	got, err := NewService(fs, nil).GetMembers(context.Background(), member, conv)
	if err != nil {
		t.Fatalf("GetMembers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("members = %d, want 2", len(got))
	}
}

func TestAddMembers_ForbiddenForMember(t *testing.T) {
	owner, member, target, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember})
	fs.userExists, fs.canMessage = true, true

	_, err := NewService(fs, nil).AddMembers(context.Background(), member, conv, []uuid.UUID{target})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestAddMembers_AdminAddsAndFans(t *testing.T) {
	owner, admin, target, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{admin: RoleAdmin})
	fs.userExists, fs.canMessage = true, true
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}

	pub := newRecordingPublisher()
	added, err := NewService(fs, pub).AddMembers(context.Background(), admin, conv, []uuid.UUID{target})
	if err != nil {
		t.Fatalf("AddMembers: %v", err)
	}
	if added != 1 || !fs.members[conv][target] {
		t.Fatalf("added=%d members=%v", added, fs.members[conv])
	}
	if !hasEvent[MemberChangedEvent](pub, owner) {
		t.Fatal("owner missing member.changed")
	}
}

func TestAddMembers_NotConnected(t *testing.T) {
	owner, admin, target, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{admin: RoleAdmin})
	fs.userExists, fs.canMessage = true, false

	_, err := NewService(fs, nil).AddMembers(context.Background(), admin, conv, []uuid.UUID{target})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
}

func TestRemoveMember_AdminRemovesMember(t *testing.T) {
	owner, admin, target, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{admin: RoleAdmin, target: RoleMember})
	fs.removed = true
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}

	if err := NewService(fs, nil).RemoveMember(context.Background(), admin, conv, target); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if fs.members[conv][target] {
		t.Fatal("target should be removed")
	}
}

func TestRemoveMember_CannotRemoveOwner(t *testing.T) {
	owner, admin, conv := uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{admin: RoleAdmin})
	err := NewService(fs, nil).RemoveMember(context.Background(), admin, conv, owner)
	if !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("want ErrOwnerImmutable, got %v", err)
	}
}

func TestRemoveMember_MemberCannotRemove(t *testing.T) {
	owner, member, target, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember, target: RoleMember})
	err := NewService(fs, nil).RemoveMember(context.Background(), member, conv, target)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestSetMemberRole_OwnerPromotes(t *testing.T) {
	owner, member, conv := uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember})
	fs.roleSet = true
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}

	if err := NewService(fs, nil).SetMemberRole(context.Background(), owner, conv, member, RoleAdmin); err != nil {
		t.Fatalf("SetMemberRole: %v", err)
	}
	if fs.roles[conv][member] != RoleAdmin {
		t.Fatalf("role = %q, want admin", fs.roles[conv][member])
	}
}

func TestSetMemberRole_NonOwnerForbidden(t *testing.T) {
	owner, admin, member, conv := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{admin: RoleAdmin, member: RoleMember})
	err := NewService(fs, nil).SetMemberRole(context.Background(), admin, conv, member, RoleAdmin)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestSetMemberRole_InvalidRole(t *testing.T) {
	owner, member, conv := uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember})
	err := NewService(fs, nil).SetMemberRole(context.Background(), owner, conv, member, RoleOwner)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestLeaveGroup_LastMemberDeletes(t *testing.T) {
	owner, conv := uuid.New(), uuid.New()
	fs := groupStore(conv, owner, nil)
	fs.leaveDeleted = true

	if err := NewService(fs, nil).LeaveGroup(context.Background(), owner, conv); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}
}

func TestLeaveGroup_PromotesNewOwner(t *testing.T) {
	owner, heir, conv := uuid.New(), uuid.New(), uuid.New()
	fs := groupStore(conv, owner, map[uuid.UUID]string{heir: RoleMember})
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}
	fs.leaveNewOwner = &heir

	pub := newRecordingPublisher()
	if err := NewService(fs, pub).LeaveGroup(context.Background(), owner, conv); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}
	if !hasEvent[MemberChangedEvent](pub, heir) {
		t.Fatal("remaining member missing member.changed")
	}
}

func TestUpdateGroup_OK(t *testing.T) {
	owner, conv := uuid.New(), uuid.New()
	title := "Renamed"
	fs := groupStore(conv, owner, nil)
	fs.updatedGroup = Conversation{ID: conv, Kind: ConversationKindGroup, Title: &title}
	fs.summary = ConversationSummary{Conversation: Conversation{ID: conv, Title: &title}}
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, MessageType: MessageTypeSystem}

	got, err := NewService(fs, nil).UpdateGroup(context.Background(), owner, conv, &title, nil)
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if got.Title == nil || *got.Title != "Renamed" {
		t.Fatalf("title = %v", got.Title)
	}
}

func TestUpdateGroup_MemberForbidden(t *testing.T) {
	owner, member, conv := uuid.New(), uuid.New(), uuid.New()
	title := "Renamed"
	fs := groupStore(conv, owner, map[uuid.UUID]string{member: RoleMember})
	_, err := NewService(fs, nil).UpdateGroup(context.Background(), member, conv, &title, nil)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestJoinNames(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"Alice":           "Alice",
		"Alice,Bob":       "Alice and Bob",
		"Alice,Bob,Carol": "Alice, Bob and Carol",
	}
	for input, want := range cases {
		var names []string
		if input != "" {
			names = strings.Split(input, ",")
		}
		if got := joinNames(names); got != want {
			t.Fatalf("joinNames(%v) = %q, want %q", names, got, want)
		}
	}
}

// --- advanced messaging (Phase 4) -------------------------------------------

// directStore builds a fake store for a direct conversation with the given
// message preloaded as the target of an action.
func directStore(conv, sender, other uuid.UUID, msg Message) *fakeStore {
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{sender: true, other: true}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindDirect}
	msg.ConversationID = conv
	fs.message = msg
	return fs
}

func TestEditMessage_NotSenderForbidden(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText, CreatedAt: time.Now()})
	_, err := NewService(fs, nil).EditMessage(context.Background(), other, conv, fs.message.ID, "new")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestEditMessage_WindowClosed(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText, CreatedAt: time.Now().Add(-time.Hour)})
	_, err := NewService(fs, nil).EditMessage(context.Background(), sender, conv, fs.message.ID, "new")
	if !errors.Is(err, ErrEditWindowClosed) {
		t.Fatalf("want ErrEditWindowClosed, got %v", err)
	}
}

func TestEditMessage_NonTextRejected(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeImage, CreatedAt: time.Now()})
	_, err := NewService(fs, nil).EditMessage(context.Background(), sender, conv, fs.message.ID, "new")
	if !errors.Is(err, ErrNotEditableType) {
		t.Fatalf("want ErrNotEditableType, got %v", err)
	}
}

func TestEditMessage_DeletedRejected(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	deleted := time.Now()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText, CreatedAt: time.Now(), DeletedAt: &deleted})
	_, err := NewService(fs, nil).EditMessage(context.Background(), sender, conv, fs.message.ID, "new")
	if !errors.Is(err, ErrCannotEditDeleted) {
		t.Fatalf("want ErrCannotEditDeleted, got %v", err)
	}
}

func TestEditMessage_OKBroadcasts(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"old"}`), CreatedAt: time.Now()})
	fs.edited = fs.message

	pub := newRecordingPublisher(sender, other)
	got, err := NewService(fs, pub).EditMessage(context.Background(), sender, conv, fs.message.ID, "new")
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if got.EditedAt == nil {
		t.Fatal("expected edited_at to be set")
	}
	var body textContent
	if err := json.Unmarshal(got.Content, &body); err != nil || body.Text != "new" {
		t.Fatalf("content = %s (err %v), want text new", got.Content, err)
	}
	for _, u := range []uuid.UUID{sender, other} {
		found := false
		for _, e := range pub.events {
			if e.user == u {
				if _, ok := e.event.(MessageUpdatedEvent); ok {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("user %s did not receive message.updated", u)
		}
	}
}

func TestDeleteMessage_SenderTombstones(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"bye"}`)})
	pub := newRecordingPublisher(sender, other)
	got, err := NewService(fs, pub).DeleteMessage(context.Background(), sender, conv, fs.message.ID)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}
	if string(got.Content) != "{}" {
		t.Fatalf("content not scrubbed: %s", got.Content)
	}
	if len(fs.deletedIDs) != 1 {
		t.Fatalf("deletedIDs = %v, want one", fs.deletedIDs)
	}
}

func TestDeleteMessage_NonSenderDirectForbidden(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText})
	_, err := NewService(fs, nil).DeleteMessage(context.Background(), other, conv, fs.message.ID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestDeleteMessage_GroupAdminAllowed(t *testing.T) {
	conv, sender, admin := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{sender: true, admin: true}
	fs.roles[conv] = map[uuid.UUID]string{sender: RoleMember, admin: RoleAdmin}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.message = Message{ID: uuid.New(), ConversationID: conv, SenderID: sender, MessageType: MessageTypeText}

	got, err := NewService(fs, nil).DeleteMessage(context.Background(), admin, conv, fs.message.ID)
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if got.DeletedAt == nil {
		t.Fatal("expected deletion")
	}
}

func TestReaction_AddAndBroadcast(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText})
	pub := newRecordingPublisher(sender, other)
	got, err := NewService(fs, pub).AddReaction(context.Background(), other, conv, fs.message.ID, "\U0001F44D")
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if len(got) != 1 || got[0].Emoji != "\U0001F44D" || !got[0].Mine || got[0].Count != 1 {
		t.Fatalf("reactions = %+v", got)
	}
	found := false
	for _, e := range pub.events {
		if ev, ok := e.event.(MessageReactionEvent); ok && ev.MessageID == fs.message.ID.String() {
			found = true
		}
	}
	if !found {
		t.Fatal("no message.reaction event broadcast")
	}

	if _, err := NewService(fs, nil).RemoveReaction(context.Background(), other, conv, fs.message.ID, "\U0001F44D"); err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if len(fs.reactions[fs.message.ID]) != 0 {
		t.Fatalf("reaction not removed: %+v", fs.reactions[fs.message.ID])
	}
}

func TestReaction_SystemMessageRejected(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeSystem})
	_, err := NewService(fs, nil).AddReaction(context.Background(), other, conv, fs.message.ID, "\U0001F44D")
	if !errors.Is(err, ErrNotReactable) {
		t.Fatalf("want ErrNotReactable, got %v", err)
	}
}

func TestSetPin_DirectAllowed(t *testing.T) {
	conv, sender, other := uuid.New(), uuid.New(), uuid.New()
	fs := directStore(conv, sender, other, Message{ID: uuid.New(), SenderID: sender, MessageType: MessageTypeText})
	if err := NewService(fs, nil).SetPin(context.Background(), other, conv, fs.message.ID, true); err != nil {
		t.Fatalf("SetPin: %v", err)
	}
	if fs.pinnedAt == nil {
		t.Fatal("expected pinned_at set")
	}
}

func TestSetPin_GroupMemberForbidden(t *testing.T) {
	conv, owner, member := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{owner: true, member: true}
	fs.roles[conv] = map[uuid.UUID]string{owner: RoleOwner, member: RoleMember}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.message = Message{ID: uuid.New(), ConversationID: conv, SenderID: owner, MessageType: MessageTypeText}
	err := NewService(fs, nil).SetPin(context.Background(), member, conv, fs.message.ID, true)
	if !errors.Is(err, ErrPinForbidden) {
		t.Fatalf("want ErrPinForbidden, got %v", err)
	}
}

func TestSetPin_GroupAdminAllowed(t *testing.T) {
	conv, owner, admin := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{owner: true, admin: true}
	fs.roles[conv] = map[uuid.UUID]string{owner: RoleOwner, admin: RoleAdmin}
	fs.groupConv = Conversation{ID: conv, Kind: ConversationKindGroup}
	fs.message = Message{ID: uuid.New(), ConversationID: conv, SenderID: owner, MessageType: MessageTypeText}
	if err := NewService(fs, nil).SetPin(context.Background(), admin, conv, fs.message.ID, true); err != nil {
		t.Fatalf("SetPin: %v", err)
	}
}

func TestForwardMessage_CopiesContent(t *testing.T) {
	source, target, me := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[source] = map[uuid.UUID]bool{me: true}
	fs.members[target] = map[uuid.UUID]bool{me: true}
	src := Message{ID: uuid.New(), ConversationID: source, SenderID: me, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"fwd"}`)}
	fs.message = src
	fs.inserted = Message{ID: uuid.New(), ConversationID: target, SenderID: me, SequenceNo: 1, MessageType: MessageTypeText, Content: src.Content}

	got, err := NewService(fs, nil).ForwardMessage(context.Background(), me, target, source, src.ID)
	if err != nil {
		t.Fatalf("ForwardMessage: %v", err)
	}
	if got.ForwardedFrom == nil || *got.ForwardedFrom != src.ID {
		t.Fatalf("forwarded_from = %v, want %s", got.ForwardedFrom, src.ID)
	}
}

func TestForwardMessage_DeletedRejected(t *testing.T) {
	source, target, me := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[source] = map[uuid.UUID]bool{me: true}
	fs.members[target] = map[uuid.UUID]bool{me: true}
	deleted := time.Now()
	fs.message = Message{ID: uuid.New(), ConversationID: source, MessageType: MessageTypeText, DeletedAt: &deleted}
	_, err := NewService(fs, nil).ForwardMessage(context.Background(), me, target, source, fs.message.ID)
	if !errors.Is(err, ErrCannotForwardDeleted) {
		t.Fatalf("want ErrCannotForwardDeleted, got %v", err)
	}
}

func TestForwardMessage_SystemRejected(t *testing.T) {
	source, target, me := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[source] = map[uuid.UUID]bool{me: true}
	fs.members[target] = map[uuid.UUID]bool{me: true}
	fs.message = Message{ID: uuid.New(), ConversationID: source, MessageType: MessageTypeSystem}
	_, err := NewService(fs, nil).ForwardMessage(context.Background(), me, target, source, fs.message.ID)
	if !errors.Is(err, ErrNotForwardable) {
		t.Fatalf("want ErrNotForwardable, got %v", err)
	}
}

func TestSearchMessages_EmptyQuery(t *testing.T) {
	conv, me := uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	_, _, err := NewService(fs, nil).SearchMessages(context.Background(), me, conv, "   ", nil, 0)
	if !errors.Is(err, ErrEmptySearch) {
		t.Fatalf("want ErrEmptySearch, got %v", err)
	}
}

func TestSearchMessages_ReturnsAscendingWithCursor(t *testing.T) {
	conv, me := uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	fs.searchResults = []Message{
		{ID: uuid.New(), SequenceNo: 2, MessageType: MessageTypeText},
		{ID: uuid.New(), SequenceNo: 5, MessageType: MessageTypeText},
	}
	fs.searchHasMore = true
	items, next, err := NewService(fs, nil).SearchMessages(context.Background(), me, conv, "hi", nil, 0)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(items) != 2 || items[0].SequenceNo != 2 {
		t.Fatalf("items = %+v", items)
	}
	if next != "2" {
		t.Fatalf("next = %q, want 2", next)
	}
}

func TestSendMessage_ReplyTargetMissing(t *testing.T) {
	conv, me := uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true}
	fs.messageErr = ErrMessageNotFound
	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"hi"}`), ReplyToID: uuid.NewString()}
	_, _, err := NewService(fs, nil).SendMessage(context.Background(), me, conv, &in)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if invalid.Fields["reply_to_id"] == "" {
		t.Fatalf("expected reply_to_id field error, got %v", invalid.Fields)
	}
}

func TestSendMessage_ReplyHydrates(t *testing.T) {
	conv, me, peer := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore()
	fs.members[conv] = map[uuid.UUID]bool{me: true, peer: true}
	original := Message{ID: uuid.New(), ConversationID: conv, SenderID: peer, SequenceNo: 1, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"orig"}`)}
	fs.message = original
	fs.messagesByID[original.ID] = original
	replyTo := original.ID
	fs.inserted = Message{ID: uuid.New(), ConversationID: conv, SenderID: me, SequenceNo: 2, MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"re"}`), ReplyToID: &replyTo}

	in := SendInput{ClientMsgID: "c1", MessageType: MessageTypeText, Content: json.RawMessage(`{"text":"re"}`), ReplyToID: original.ID.String()}
	got, _, err := NewService(fs, nil).SendMessage(context.Background(), me, conv, &in)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got.ReplyTo == nil || got.ReplyTo.ID != original.ID {
		t.Fatalf("reply not hydrated: %+v", got.ReplyTo)
	}
}
