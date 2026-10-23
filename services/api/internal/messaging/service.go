package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Publisher delivers an event to every live connection of a user. It reports
// whether at least one connection received the event, which lets the service
// advance delivery state without waiting for an ack round trip.
type Publisher interface {
	PublishToUser(userID uuid.UUID, event any) bool
}

// store is the persistence surface the service depends on. It is an interface
// so the service can be unit-tested without a database.
type store interface {
	ListConversations(ctx context.Context, me uuid.UUID, after *ConversationCursor, limit int) ([]ConversationSummary, error)
	GetConversationSummary(ctx context.Context, me, id uuid.UUID) (ConversationSummary, error)
	IsMember(ctx context.Context, conversationID, userID uuid.UUID) (bool, error)
	MemberIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error)
	MemberState(ctx context.Context, conversationID, userID uuid.UUID) (MemberReadState, error)
	FindOrCreateDirect(ctx context.Context, a, b uuid.UUID) (Conversation, bool, error)
	UserExists(ctx context.Context, id uuid.UUID) (bool, error)
	CanMessage(ctx context.Context, a, b uuid.UUID) (bool, error)
	InsertMessage(ctx context.Context, p InsertMessageParams) (Message, bool, error)
	ListMessages(ctx context.Context, conversationID, viewer uuid.UUID, q MessageQuery) ([]Message, bool, error)
	StatusFor(ctx context.Context, conversationID, viewer, sender uuid.UUID, sequence int64) (string, error)
	AdvanceRead(ctx context.Context, conversationID, userID uuid.UUID, upTo int64) (int64, error)
	AdvanceDelivered(ctx context.Context, conversationID, userID uuid.UUID, upTo int64) (int64, error)
	AdvanceDeliveredForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	UnreadCount(ctx context.Context, conversationID, reader uuid.UUID, readSequence int64) (int64, error)
	PeekSequence(ctx context.Context, conversationID uuid.UUID) (int64, error)
	// Group operations (Phase 4).
	Conversation(ctx context.Context, id uuid.UUID) (Conversation, error)
	ProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error)
	MemberRole(ctx context.Context, conversationID, userID uuid.UUID) (string, error)
	CountMembers(ctx context.Context, conversationID uuid.UUID) (int64, error)
	MemberDetails(ctx context.Context, conversationID uuid.UUID) ([]MemberDetail, error)
	CreateGroup(ctx context.Context, owner uuid.UUID, title string, memberIDs []uuid.UUID) (Conversation, error)
	UpdateGroupMeta(ctx context.Context, id uuid.UUID, title, avatarURL *string) (Conversation, error)
	AddMembers(ctx context.Context, id uuid.UUID, userIDs []uuid.UUID) (int64, error)
	RemoveMember(ctx context.Context, id, userID uuid.UUID) (bool, error)
	SetMemberRole(ctx context.Context, id, userID uuid.UUID, role string) (bool, error)
	LeaveGroup(ctx context.Context, id, userID uuid.UUID) (bool, *uuid.UUID, error)
	// Advanced messaging (Phase 4).
	MessageFor(ctx context.Context, conversationID, messageID uuid.UUID) (Message, error)
	MessagesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Message, error)
	EditMessage(ctx context.Context, messageID uuid.UUID, content json.RawMessage, editedAt time.Time) (Message, error)
	SoftDeleteMessage(ctx context.Context, messageID uuid.UUID, deletedAt time.Time) error
	AddReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string) (bool, error)
	RemoveReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string) (bool, error)
	ReactionsFor(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]Reaction, error)
	PinMessage(ctx context.Context, messageID uuid.UUID, pinnedAt *time.Time) error
	PinnedMessages(ctx context.Context, conversationID uuid.UUID) ([]Message, error)
	SearchMessages(ctx context.Context, conversationID, viewer uuid.UUID, pattern string, before *int64, limit int) ([]Message, bool, error)
}

// Page-size bounds for message history.
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// editWindow is how long after sending a text message may still be edited.
const editWindow = 15 * time.Minute

// Service is the messaging use-case layer shared by the REST handlers and the
// realtime transport. It owns authorization, validation, and fan-out.
type Service struct {
	store store
	pub   Publisher
	now   func() time.Time
}

// NewService builds a messaging Service. pub may be nil, in which case events
// are simply not broadcast (useful in REST-only tests).
func NewService(st store, pub Publisher) *Service {
	return &Service{store: st, pub: pub, now: time.Now}
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultPageLimit
	case limit > maxPageLimit:
		return maxPageLimit
	default:
		return limit
	}
}

// activityOf is the sort key for a conversation: the newest message time, or
// its creation time before any message exists.
func activityOf(s ConversationSummary) time.Time {
	if s.LastMessageAt != nil {
		return *s.LastMessageAt
	}
	return s.CreatedAt
}

// ListConversations returns a page of the caller's conversations, newest
// activity first, plus the cursor for the next page ("" when exhausted).
func (s *Service) ListConversations(ctx context.Context, me uuid.UUID, after *ConversationCursor, limit int) ([]ConversationSummary, string, error) {
	limit = clampLimit(limit)
	items, err := s.store.ListConversations(ctx, me, after, limit+1)
	if err != nil {
		return nil, "", err
	}

	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = ConversationCursor{At: activityOf(last), ID: last.ID}.String()
	}
	return items, next, nil
}

// GetConversation returns one conversation summary for the caller.
func (s *Service) GetConversation(ctx context.Context, me, id uuid.UUID) (ConversationSummary, error) {
	return s.store.GetConversationSummary(ctx, me, id)
}

// Members returns the member ids of a conversation the caller belongs to. It
// backs transport-level fan-out (typing indicators) that does not touch
// persisted state.
func (s *Service) Members(ctx context.Context, me, conversationID uuid.UUID) ([]uuid.UUID, error) {
	member, err := s.store.IsMember(ctx, conversationID, me)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, ErrConversationNotFound
	}
	return s.store.MemberIDs(ctx, conversationID)
}

// OpenConversation finds or creates the direct conversation between the caller
// and a peer they are connected to and have not been blocked by.
func (s *Service) OpenConversation(ctx context.Context, me, peer uuid.UUID) (ConversationSummary, error) {
	if me == peer {
		return ConversationSummary{}, ErrSelfConversation
	}
	exists, err := s.store.UserExists(ctx, peer)
	if err != nil {
		return ConversationSummary{}, err
	}
	if !exists {
		return ConversationSummary{}, ErrPeerNotFound
	}
	allowed, err := s.store.CanMessage(ctx, me, peer)
	if err != nil {
		return ConversationSummary{}, err
	}
	if !allowed {
		return ConversationSummary{}, ErrBlocked
	}

	conv, _, err := s.store.FindOrCreateDirect(ctx, me, peer)
	if err != nil {
		return ConversationSummary{}, err
	}
	return s.store.GetConversationSummary(ctx, me, conv.ID)
}

// ListMessages returns a page of history oldest-first. `before` walks backwards
// through history; `after` is the reconnect catch-up path. The returned cursor
// is a sequence number for the next page, or "" when exhausted.
func (s *Service) ListMessages(ctx context.Context, me, conversationID uuid.UUID, q MessageQuery) ([]Message, string, error) {
	member, err := s.store.IsMember(ctx, conversationID, me)
	if err != nil {
		return nil, "", err
	}
	if !member {
		return nil, "", ErrConversationNotFound
	}
	if q.Before != nil && q.After != nil {
		return nil, "", ErrInvalidSequence
	}

	q.Limit = clampLimit(q.Limit)
	items, hasMore, err := s.store.ListMessages(ctx, conversationID, me, q)
	if err != nil {
		return nil, "", err
	}
	items = s.hydrate(ctx, items)

	next := ""
	if hasMore && len(items) > 0 {
		if q.After != nil {
			next = strconv.FormatInt(items[len(items)-1].SequenceNo, 10)
		} else {
			next = strconv.FormatInt(items[0].SequenceNo, 10)
		}
	}
	return items, next, nil
}

// ensureMember returns ErrConversationNotFound when the caller is not a member,
// so membership cannot be probed through a message endpoint.
func (s *Service) ensureMember(ctx context.Context, conversationID, me uuid.UUID) error {
	member, err := s.store.IsMember(ctx, conversationID, me)
	if err != nil {
		return err
	}
	if !member {
		return ErrConversationNotFound
	}
	return nil
}

// hydrate attaches reply previews and reaction aggregates to a page of
// messages. Reply targets are batch-loaded (one query) and reactions for the
// whole page (one query), so a page costs two extra queries regardless of size.
func (s *Service) hydrate(ctx context.Context, msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}
	ids := make([]uuid.UUID, 0, len(msgs))
	replyIDs := make([]uuid.UUID, 0, len(msgs))
	seen := make(map[uuid.UUID]bool, len(msgs))
	for i := range msgs {
		ids = append(ids, msgs[i].ID)
		if msgs[i].ReplyToID != nil && !seen[*msgs[i].ReplyToID] {
			seen[*msgs[i].ReplyToID] = true
			replyIDs = append(replyIDs, *msgs[i].ReplyToID)
		}
	}
	if len(replyIDs) > 0 {
		if originals, err := s.store.MessagesByIDs(ctx, replyIDs); err == nil {
			for i := range msgs {
				if msgs[i].ReplyToID == nil {
					continue
				}
				if original, ok := originals[*msgs[i].ReplyToID]; ok {
					o := original
					msgs[i].ReplyTo = &o
				}
			}
		}
	}
	if reactions, err := s.store.ReactionsFor(ctx, ids); err == nil {
		for i := range msgs {
			if rs, ok := reactions[msgs[i].ID]; ok {
				msgs[i].Reactions = rs
			}
		}
	}
	return msgs
}

// hydrateOne hydrates a single message (used after send/edit/forward).
func (s *Service) hydrateOne(ctx context.Context, m Message) Message {
	hydrated := s.hydrate(ctx, []Message{m})
	return hydrated[0]
}

// broadcast publishes an event to every member of a conversation. The actor is
// included: their own client merges by message id, and their other tabs need it.
func (s *Service) broadcast(ctx context.Context, conversationID uuid.UUID, event any) {
	if s.pub == nil {
		return
	}
	members, err := s.store.MemberIDs(ctx, conversationID)
	if err != nil {
		return
	}
	for _, member := range members {
		s.pub.PublishToUser(member, event)
	}
}

// SyncMessages returns messages strictly newer than `after`, oldest-first, for a
// client reconnecting after a disconnect. It caps at one page; callers repeat
// with the returned cursor until it is empty.
func (s *Service) SyncMessages(ctx context.Context, me, conversationID uuid.UUID, after int64) ([]Message, string, error) {
	cursor := after
	return s.ListMessages(ctx, me, conversationID, MessageQuery{After: &cursor, Limit: maxPageLimit})
}

// SendMessage validates, persists, and fans out a message. The bool reports
// whether this was an idempotent replay of a previously stored client_msg_id.
func (s *Service) SendMessage(ctx context.Context, me, conversationID uuid.UUID, in *SendInput) (Message, bool, error) {
	fields := in.Validate()
	if len(fields) > 0 {
		return Message{}, false, &ValidationError{Fields: fields}
	}

	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return Message{}, false, err
	}

	members, err := s.store.MemberIDs(ctx, conversationID)
	if err != nil {
		return Message{}, false, err
	}

	// Resolve an optional reply target, scoped to this conversation.
	var replyTo *uuid.UUID
	if in.ReplyToID != "" {
		rid, _ := uuid.Parse(in.ReplyToID)
		if _, err := s.store.MessageFor(ctx, conversationID, rid); err != nil {
			if errors.Is(err, ErrMessageNotFound) {
				return Message{}, false, &ValidationError{Fields: map[string]string{
					"reply_to_id": "That message does not exist in this conversation.",
				}}
			}
			return Message{}, false, err
		}
		replyTo = &rid
	}

	clientID := in.ClientMsgID
	msg, duplicate, err := s.store.InsertMessage(ctx, InsertMessageParams{
		ConversationID: conversationID,
		SenderID:       me,
		ClientMsgID:    &clientID,
		MessageType:    in.MessageType,
		Content:        in.Content,
		ReplyToID:      replyTo,
	})
	if err != nil {
		return Message{}, false, err
	}

	// Compute the sender's own view of the new message for the REST response.
	if status, err := s.store.StatusFor(ctx, conversationID, me, me, msg.SequenceNo); err == nil {
		msg.Status = status
	} else {
		msg.Status = StatusSent
	}
	msg = s.hydrateOne(ctx, msg)

	if !duplicate {
		s.fanOutCreated(ctx, conversationID, msg, members, me)
	}
	return msg, duplicate, nil
}

// fanOutCreated publishes message.created to every member and advances delivery
// for members with a live connection, notifying the sender with receipts.
func (s *Service) fanOutCreated(ctx context.Context, conversationID uuid.UUID, msg Message, members []uuid.UUID, sender uuid.UUID) {
	if s.pub == nil {
		return
	}

	delivered := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		view := msg
		if member != sender {
			// A freshly delivered message is unread from the recipient's side.
			view.Status = StatusReceived
		}
		connected := s.pub.PublishToUser(member, MessageCreatedEvent{
			Type:           EventMessageCreated,
			EventID:        uuid.NewString(),
			ConversationID: conversationID.String(),
			Message:        ToMessageResponseFor(view, member),
		})
		if connected && member != sender {
			delivered = append(delivered, member)
		}
	}

	for _, member := range delivered {
		seq, err := s.store.AdvanceDelivered(ctx, conversationID, member, msg.SequenceNo)
		if err != nil {
			continue
		}
		s.pub.PublishToUser(sender, MessageReceiptEvent{
			Type:           EventMessageReceipt,
			ConversationID: conversationID.String(),
			UserID:         member.String(),
			Status:         StatusDelivered,
			UpToSequence:   seq,
		})
	}
}

// MarkRead advances the caller's read position and notifies the other members.
// It returns the caller's remaining unread count.
func (s *Service) MarkRead(ctx context.Context, me, conversationID uuid.UUID, upTo int64) (int64, error) {
	member, err := s.store.IsMember(ctx, conversationID, me)
	if err != nil {
		return 0, err
	}
	if !member {
		return 0, ErrConversationNotFound
	}
	if upTo < 0 {
		return 0, ErrInvalidSequence
	}

	// Reading past the end of history is harmless; clamp so the stored position
	// never exceeds reality.
	if max, err := s.store.PeekSequence(ctx, conversationID); err == nil && upTo > max {
		upTo = max
	}

	seq, err := s.store.AdvanceRead(ctx, conversationID, me, upTo)
	if err != nil {
		return 0, err
	}

	unread, err := s.store.UnreadCount(ctx, conversationID, me, seq)
	if err != nil {
		return 0, err
	}

	s.publishReceipt(ctx, conversationID, me, StatusRead, seq)
	return unread, nil
}

// MarkDelivered advances the caller's delivered position to the latest message
// and notifies the senders. It is a no-op when already current.
func (s *Service) MarkDelivered(ctx context.Context, me, conversationID uuid.UUID) error {
	member, err := s.store.IsMember(ctx, conversationID, me)
	if err != nil {
		return err
	}
	if !member {
		return ErrConversationNotFound
	}

	max, err := s.store.PeekSequence(ctx, conversationID)
	if err != nil {
		return err
	}

	seq, err := s.store.AdvanceDelivered(ctx, conversationID, me, max)
	if err != nil {
		if errors.Is(err, ErrConversationNotFound) {
			return nil // already current
		}
		return err
	}

	s.publishReceipt(ctx, conversationID, me, StatusDelivered, seq)
	return nil
}

// HandleConnect advances delivery for every conversation of a user who just
// came online and notifies the senders, so messages sent while the user was
// away immediately read as delivered.
func (s *Service) HandleConnect(ctx context.Context, me uuid.UUID) error {
	if s.pub == nil {
		return nil
	}
	conversationIDs, err := s.store.AdvanceDeliveredForUser(ctx, me)
	if err != nil {
		return err
	}

	for _, id := range conversationIDs {
		members, err := s.store.MemberIDs(ctx, id)
		if err != nil {
			continue
		}
		seq, err := s.store.PeekSequence(ctx, id)
		if err != nil {
			continue
		}
		for _, member := range members {
			if member == me {
				continue
			}
			s.pub.PublishToUser(member, MessageReceiptEvent{
				Type:           EventMessageReceipt,
				ConversationID: id.String(),
				UserID:         me.String(),
				Status:         StatusDelivered,
				UpToSequence:   seq,
			})
		}
	}
	return nil
}

// publishReceipt tells every other member of a conversation that `actor`
// advanced its delivered/read position.
func (s *Service) publishReceipt(ctx context.Context, conversationID, actor uuid.UUID, status string, upTo int64) {
	if s.pub == nil {
		return
	}
	members, err := s.store.MemberIDs(ctx, conversationID)
	if err != nil {
		return
	}
	event := MessageReceiptEvent{
		Type:           EventMessageReceipt,
		ConversationID: conversationID.String(),
		UserID:         actor.String(),
		Status:         status,
		UpToSequence:   upTo,
	}
	for _, member := range members {
		if member == actor {
			continue
		}
		s.pub.PublishToUser(member, event)
	}
}

// --- groups -----------------------------------------------------------------

// loadGroup loads a group conversation and the caller's role in it. A direct
// conversation yields ErrNotGroup; a non-member yields ErrConversationNotFound
// so group membership cannot be probed.
func (s *Service) loadGroup(ctx context.Context, id, me uuid.UUID) (Conversation, string, error) {
	conv, err := s.store.Conversation(ctx, id)
	if err != nil {
		return Conversation{}, "", err
	}
	if conv.Kind != ConversationKindGroup {
		return Conversation{}, "", ErrNotGroup
	}
	role, err := s.store.MemberRole(ctx, id, me)
	if errors.Is(err, ErrNotMember) {
		return Conversation{}, "", ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, "", err
	}
	return conv, role, nil
}

// displayName returns a user's display name, falling back to "Someone" so a
// system message is never blank.
func (s *Service) displayName(ctx context.Context, id uuid.UUID) string {
	p, err := s.store.ProfileBrief(ctx, id)
	if err != nil || p.DisplayName == "" {
		return "Someone"
	}
	return p.DisplayName
}

// dedupeUsers removes duplicates, the zero uuid, and an optional exclusion.
func dedupeUsers(ids []uuid.UUID, exclude uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || id == exclude || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// postSystem posts a server-generated system message and fans it out. The
// acting user is recorded as the sender so the message can still be rendered if
// that user later leaves.
func (s *Service) postSystem(ctx context.Context, conversationID, actor uuid.UUID, text string) {
	content := mustMarshal(textContent{Text: text})
	msg, _, err := s.store.InsertMessage(ctx, InsertMessageParams{
		ConversationID: conversationID,
		SenderID:       actor,
		MessageType:    MessageTypeSystem,
		Content:        content,
	})
	if err != nil {
		return
	}
	members, err := s.store.MemberIDs(ctx, conversationID)
	if err != nil {
		return
	}
	s.fanOutCreated(ctx, conversationID, msg, members, actor)
}

// publishConversationUpdated tells recipients that a group's metadata or member
// count changed.
func (s *Service) publishConversationUpdated(ctx context.Context, id uuid.UUID, recipients []uuid.UUID) {
	if s.pub == nil {
		return
	}
	conv, err := s.store.Conversation(ctx, id)
	if err != nil {
		return
	}
	count, err := s.store.CountMembers(ctx, id)
	if err != nil {
		return
	}
	event := ConversationUpdatedEvent{
		Type:           EventConversationUpdated,
		ConversationID: id.String(),
		Title:          conv.Title,
		AvatarURL:      conv.AvatarURL,
		MemberCount:    count,
	}
	for _, r := range recipients {
		s.pub.PublishToUser(r, event)
	}
}

// publishMemberChanged tells recipients about a membership change.
func (s *Service) publishMemberChanged(ctx context.Context, id, user uuid.UUID, role, action string, recipients []uuid.UUID) {
	if s.pub == nil {
		return
	}
	event := MemberChangedEvent{
		Type:           EventMemberChanged,
		ConversationID: id.String(),
		UserID:         user.String(),
		Role:           role,
		Action:         action,
	}
	for _, r := range recipients {
		s.pub.PublishToUser(r, event)
	}
}

// CreateGroup creates a group owned by the caller. Every invited member must be
// an accepted contact of the caller and must not be blocked.
func (s *Service) CreateGroup(ctx context.Context, me uuid.UUID, title string, memberIDs []uuid.UUID) (ConversationSummary, error) {
	invitees := dedupeUsers(memberIDs, me)
	if len(invitees) > maxGroupMembers-1 {
		return ConversationSummary{}, &ValidationError{Fields: map[string]string{
			"member_ids": "Too many members.",
		}}
	}
	for _, target := range invitees {
		exists, err := s.store.UserExists(ctx, target)
		if err != nil {
			return ConversationSummary{}, err
		}
		if !exists {
			return ConversationSummary{}, ErrPeerNotFound
		}
		allowed, err := s.store.CanMessage(ctx, me, target)
		if err != nil {
			return ConversationSummary{}, err
		}
		if !allowed {
			return ConversationSummary{}, ErrNotConnected
		}
	}

	conv, err := s.store.CreateGroup(ctx, me, title, invitees)
	if err != nil {
		return ConversationSummary{}, err
	}

	s.postSystem(ctx, conv.ID, me, fmt.Sprintf("%s created the group \u201c%s\u201d.", s.displayName(ctx, me), title))

	recipients := append([]uuid.UUID{me}, invitees...)
	s.publishConversationUpdated(ctx, conv.ID, recipients)
	for _, target := range invitees {
		s.publishMemberChanged(ctx, conv.ID, target, RoleMember, MemberActionAdded, recipients)
	}

	return s.store.GetConversationSummary(ctx, me, conv.ID)
}

// GetMembers lists a group's members with their roles.
func (s *Service) GetMembers(ctx context.Context, me, id uuid.UUID) ([]MemberDetail, error) {
	if _, _, err := s.loadGroup(ctx, id, me); err != nil {
		return nil, err
	}
	return s.store.MemberDetails(ctx, id)
}

// AddMembers adds accepted contacts to a group. Callers must be an owner or
// admin. Existing members are ignored; unknown or unconnected users are
// rejected.
func (s *Service) AddMembers(ctx context.Context, me, id uuid.UUID, userIDs []uuid.UUID) (int64, error) {
	_, role, err := s.loadGroup(ctx, id, me)
	if err != nil {
		return 0, err
	}
	if roleRank(role) < roleRank(RoleAdmin) {
		return 0, ErrForbidden
	}

	toAdd := make([]uuid.UUID, 0, len(userIDs))
	for _, target := range dedupeUsers(userIDs, me) {
		exists, err := s.store.UserExists(ctx, target)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, ErrPeerNotFound
		}
		member, err := s.store.IsMember(ctx, id, target)
		if err != nil {
			return 0, err
		}
		if member {
			continue
		}
		allowed, err := s.store.CanMessage(ctx, me, target)
		if err != nil {
			return 0, err
		}
		if !allowed {
			return 0, ErrNotConnected
		}
		toAdd = append(toAdd, target)
	}

	if len(toAdd) == 0 {
		return 0, nil
	}

	added, err := s.store.AddMembers(ctx, id, toAdd)
	if err != nil {
		return 0, err
	}
	if added == 0 {
		return 0, nil
	}

	names := make([]string, 0, len(toAdd))
	for _, target := range toAdd {
		names = append(names, s.displayName(ctx, target))
	}
	s.postSystem(ctx, id, me, fmt.Sprintf("%s added %s.", s.displayName(ctx, me), joinNames(names)))

	members, err := s.store.MemberIDs(ctx, id)
	if err != nil {
		return added, err
	}
	for _, target := range toAdd {
		s.publishMemberChanged(ctx, id, target, RoleMember, MemberActionAdded, members)
	}
	s.publishConversationUpdated(ctx, id, members)
	return added, nil
}

// RemoveMember removes a member from a group. The caller must outrank the
// target: an owner may remove admins and members, an admin may remove members.
// The owner cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, me, id, target uuid.UUID) error {
	_, role, err := s.loadGroup(ctx, id, me)
	if err != nil {
		return err
	}
	if target == me {
		return ErrForbidden
	}
	targetRole, err := s.store.MemberRole(ctx, id, target)
	if err != nil {
		return err
	}
	if targetRole == RoleOwner {
		return ErrOwnerImmutable
	}
	if roleRank(role) <= roleRank(targetRole) {
		return ErrForbidden
	}

	removed, err := s.store.RemoveMember(ctx, id, target)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotMember
	}

	s.postSystem(ctx, id, me, fmt.Sprintf("%s removed %s.", s.displayName(ctx, me), s.displayName(ctx, target)))

	members, err := s.store.MemberIDs(ctx, id)
	if err != nil {
		return err
	}
	// Notify remaining members and the removed user (so their client updates).
	recipients := append(append([]uuid.UUID{}, members...), target)
	s.publishMemberChanged(ctx, id, target, RoleMember, MemberActionRemoved, recipients)
	s.publishConversationUpdated(ctx, id, members)
	return nil
}

// SetMemberRole promotes or demotes a member. Only the owner may do this, and
// the owner's own role is immutable.
func (s *Service) SetMemberRole(ctx context.Context, me, id, target uuid.UUID, role string) error {
	if role != RoleAdmin && role != RoleMember {
		return &ValidationError{Fields: map[string]string{"role": "Role must be admin or member."}}
	}
	_, myRole, err := s.loadGroup(ctx, id, me)
	if err != nil {
		return err
	}
	if myRole != RoleOwner {
		return ErrForbidden
	}
	targetRole, err := s.store.MemberRole(ctx, id, target)
	if err != nil {
		return err
	}
	if targetRole == RoleOwner {
		return ErrOwnerImmutable
	}
	if _, err := s.store.SetMemberRole(ctx, id, target, role); err != nil {
		return err
	}

	if role == RoleAdmin {
		s.postSystem(ctx, id, me, fmt.Sprintf("%s made %s an admin.", s.displayName(ctx, me), s.displayName(ctx, target)))
	} else {
		s.postSystem(ctx, id, me, fmt.Sprintf("%s removed %s as an admin.", s.displayName(ctx, me), s.displayName(ctx, target)))
	}

	members, err := s.store.MemberIDs(ctx, id)
	if err != nil {
		return err
	}
	s.publishMemberChanged(ctx, id, target, role, MemberActionRole, members)
	return nil
}

// LeaveGroup removes the caller from a group. Ownership passes to the earliest
// remaining member; the last member leaving deletes the conversation.
func (s *Service) LeaveGroup(ctx context.Context, me, id uuid.UUID) error {
	if _, _, err := s.loadGroup(ctx, id, me); err != nil {
		return err
	}

	name := s.displayName(ctx, me)
	deleted, newOwner, err := s.store.LeaveGroup(ctx, id, me)
	if err != nil {
		return err
	}
	if deleted {
		return nil
	}

	s.postSystem(ctx, id, me, fmt.Sprintf("%s left the group.", name))

	members, err := s.store.MemberIDs(ctx, id)
	if err != nil {
		return err
	}
	s.publishMemberChanged(ctx, id, me, RoleMember, MemberActionLeft, members)
	if newOwner != nil {
		s.publishMemberChanged(ctx, id, *newOwner, RoleOwner, MemberActionRole, members)
	}
	s.publishConversationUpdated(ctx, id, members)
	return nil
}

// UpdateGroup changes a group's title and/or avatar. Callers must be an owner
// or admin.
func (s *Service) UpdateGroup(ctx context.Context, me, id uuid.UUID, title, avatarURL *string) (ConversationSummary, error) {
	_, role, err := s.loadGroup(ctx, id, me)
	if err != nil {
		return ConversationSummary{}, err
	}
	if roleRank(role) < roleRank(RoleAdmin) {
		return ConversationSummary{}, ErrForbidden
	}

	if _, err := s.store.UpdateGroupMeta(ctx, id, title, avatarURL); err != nil {
		return ConversationSummary{}, err
	}

	actor := s.displayName(ctx, me)
	switch {
	case title != nil:
		s.postSystem(ctx, id, me, fmt.Sprintf("%s changed the group name to \u201c%s\u201d.", actor, *title))
	case avatarURL != nil:
		s.postSystem(ctx, id, me, fmt.Sprintf("%s changed the group photo.", actor))
	}

	members, err := s.store.MemberIDs(ctx, id)
	if err != nil {
		return ConversationSummary{}, err
	}
	s.publishConversationUpdated(ctx, id, members)

	return s.store.GetConversationSummary(ctx, me, id)
}

// joinNames renders a comma-separated list, using "and" for the final element.
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		out := ""
		for i, n := range names {
			switch i {
			case len(names) - 1:
				out += " and " + n
			case 0:
				out = n
			default:
				out += ", " + n
			}
		}
		return out
	}
}

// --- advanced messaging (Phase 4) -------------------------------------------

// broadcastViewer publishes one event per member, letting the payload depend on
// the recipient (used where a field such as reaction "mine" is viewer-relative).
func (s *Service) broadcastViewer(ctx context.Context, conversationID uuid.UUID, makeEvent func(viewer uuid.UUID) any) {
	if s.pub == nil {
		return
	}
	members, err := s.store.MemberIDs(ctx, conversationID)
	if err != nil {
		return
	}
	for _, member := range members {
		s.pub.PublishToUser(member, makeEvent(member))
	}
}

// EditMessage rewrites a text message's body. Only the sender may edit it, only
// text messages that are not deleted, and only within editWindow of sending.
func (s *Service) EditMessage(ctx context.Context, me, conversationID, messageID uuid.UUID, text string) (Message, error) {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return Message{}, err
	}
	msg, err := s.store.MessageFor(ctx, conversationID, messageID)
	if err != nil {
		return Message{}, err
	}
	if msg.SenderID != me {
		return Message{}, ErrForbidden
	}
	switch {
	case msg.DeletedAt != nil:
		return Message{}, ErrCannotEditDeleted
	case msg.MessageType != MessageTypeText:
		return Message{}, ErrNotEditableType
	case s.now().Sub(msg.CreatedAt) > editWindow:
		return Message{}, ErrEditWindowClosed
	}

	updated, err := s.store.EditMessage(ctx, messageID, mustMarshal(textContent{Text: text}), s.now())
	if err != nil {
		return Message{}, err
	}
	updated = s.hydrateOne(ctx, updated)
	s.broadcastViewer(ctx, conversationID, func(viewer uuid.UUID) any {
		return MessageUpdatedEvent{
			Type:           EventMessageUpdated,
			ConversationID: conversationID.String(),
			Message:        ToMessageResponseFor(updated, viewer),
		}
	})
	return updated, nil
}

// DeleteMessage tombstones a message. The sender may always delete their own
// message; in a group an owner or admin may also delete another member's.
func (s *Service) DeleteMessage(ctx context.Context, me, conversationID, messageID uuid.UUID) (Message, error) {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return Message{}, err
	}
	msg, err := s.store.MessageFor(ctx, conversationID, messageID)
	if err != nil {
		return Message{}, err
	}
	if msg.DeletedAt != nil {
		return Message{}, ErrMessageNotFound
	}
	if msg.SenderID != me {
		conv, err := s.store.Conversation(ctx, conversationID)
		if err != nil {
			return Message{}, err
		}
		if conv.Kind != ConversationKindGroup {
			return Message{}, ErrForbidden
		}
		role, err := s.store.MemberRole(ctx, conversationID, me)
		if err != nil {
			return Message{}, err
		}
		if roleRank(role) < roleRank(RoleAdmin) {
			return Message{}, ErrForbidden
		}
	}

	now := s.now()
	if err := s.store.SoftDeleteMessage(ctx, messageID, now); err != nil {
		return Message{}, err
	}
	msg.Content = json.RawMessage(`{}`)
	msg.DeletedAt = &now
	msg.Reactions = nil
	msg.ReplyTo = nil

	s.broadcast(ctx, conversationID, MessageDeletedEvent{
		Type:           EventMessageDeleted,
		ConversationID: conversationID.String(),
		MessageID:      messageID.String(),
		Message:        ToMessageResponse(msg),
	})
	return msg, nil
}

// AddReaction records the caller's emoji reaction on a message and returns the
// updated aggregate as the caller sees it.
func (s *Service) AddReaction(ctx context.Context, me, conversationID, messageID uuid.UUID, emoji string) ([]ReactionResponse, error) {
	return s.setReaction(ctx, me, conversationID, messageID, emoji, true)
}

// RemoveReaction removes the caller's emoji reaction from a message.
func (s *Service) RemoveReaction(ctx context.Context, me, conversationID, messageID uuid.UUID, emoji string) ([]ReactionResponse, error) {
	return s.setReaction(ctx, me, conversationID, messageID, emoji, false)
}

func (s *Service) setReaction(ctx context.Context, me, conversationID, messageID uuid.UUID, emoji string, add bool) ([]ReactionResponse, error) {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return nil, err
	}
	msg, err := s.store.MessageFor(ctx, conversationID, messageID)
	if err != nil {
		return nil, err
	}
	if msg.MessageType == MessageTypeSystem {
		return nil, ErrNotReactable
	}
	if msg.DeletedAt != nil {
		return nil, ErrMessageNotFound
	}

	if add {
		if _, err := s.store.AddReaction(ctx, messageID, me, emoji); err != nil {
			return nil, err
		}
	} else {
		if _, err := s.store.RemoveReaction(ctx, messageID, me, emoji); err != nil {
			return nil, err
		}
	}

	reactions, err := s.store.ReactionsFor(ctx, []uuid.UUID{messageID})
	if err != nil {
		return nil, err
	}
	raw := reactions[messageID]

	s.broadcastViewer(ctx, conversationID, func(viewer uuid.UUID) any {
		return MessageReactionEvent{
			Type:           EventMessageReaction,
			ConversationID: conversationID.String(),
			MessageID:      messageID.String(),
			ActorID:        me.String(),
			Reactions:      reactionsResponse(raw, viewer),
		}
	})
	return reactionsResponse(raw, me), nil
}

// SetPin pins or unpins a message. In a direct conversation either member may
// pin; in a group the owner or an admin must.
func (s *Service) SetPin(ctx context.Context, me, conversationID, messageID uuid.UUID, pinned bool) error {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return err
	}
	msg, err := s.store.MessageFor(ctx, conversationID, messageID)
	if err != nil {
		return err
	}
	if msg.DeletedAt != nil {
		return ErrMessageNotFound
	}
	conv, err := s.store.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.Kind == ConversationKindGroup {
		role, err := s.store.MemberRole(ctx, conversationID, me)
		if err != nil {
			return err
		}
		if roleRank(role) < roleRank(RoleAdmin) {
			return ErrPinForbidden
		}
	}

	var at *time.Time
	if pinned {
		t := s.now()
		at = &t
	}
	if err := s.store.PinMessage(ctx, messageID, at); err != nil {
		return err
	}

	s.broadcast(ctx, conversationID, MessagePinnedEvent{
		Type:           EventMessagePinned,
		ConversationID: conversationID.String(),
		MessageID:      messageID.String(),
		ActorID:        me.String(),
		Pinned:         pinned,
	})
	return nil
}

// PinnedMessages lists the pinned messages in a conversation the caller is in.
func (s *Service) PinnedMessages(ctx context.Context, me, conversationID uuid.UUID) ([]Message, error) {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return nil, err
	}
	msgs, err := s.store.PinnedMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, msgs), nil
}

// ForwardMessage copies a message's content into the destination conversation
// (the :id of the route), recording the origin. The caller must be a member of
// both conversations and the source must not be deleted.
func (s *Service) ForwardMessage(ctx context.Context, me, targetID, sourceID, messageID uuid.UUID) (Message, error) {
	if err := s.ensureMember(ctx, sourceID, me); err != nil {
		return Message{}, err
	}
	if err := s.ensureMember(ctx, targetID, me); err != nil {
		return Message{}, err
	}
	src, err := s.store.MessageFor(ctx, sourceID, messageID)
	if err != nil {
		return Message{}, err
	}
	if src.DeletedAt != nil {
		return Message{}, ErrCannotForwardDeleted
	}
	if src.MessageType == MessageTypeSystem {
		return Message{}, ErrNotForwardable
	}

	origin := src.ID
	msg, _, err := s.store.InsertMessage(ctx, InsertMessageParams{
		ConversationID: targetID,
		SenderID:       me,
		MessageType:    src.MessageType,
		Content:        src.Content,
		ForwardedFrom:  &origin,
	})
	if err != nil {
		return Message{}, err
	}

	if status, err := s.store.StatusFor(ctx, targetID, me, me, msg.SequenceNo); err == nil {
		msg.Status = status
	} else {
		msg.Status = StatusSent
	}
	msg = s.hydrateOne(ctx, msg)

	members, err := s.store.MemberIDs(ctx, targetID)
	if err == nil {
		s.fanOutCreated(ctx, targetID, msg, members, me)
	}
	return msg, nil
}

// SearchMessages finds text messages in a conversation matching a query. It
// returns matches in ascending sequence order and the `before` cursor (a
// sequence number, as a string) for loading older matches.
func (s *Service) SearchMessages(ctx context.Context, me, conversationID uuid.UUID, query string, before *int64, limit int) ([]Message, string, error) {
	if err := s.ensureMember(ctx, conversationID, me); err != nil {
		return nil, "", err
	}
	q := trimSpace(query)
	if q == "" {
		return nil, "", ErrEmptySearch
	}
	limit = clampLimit(limit)

	items, hasMore, err := s.store.SearchMessages(ctx, conversationID, me, "%"+escapeLike(q)+"%", before, limit)
	if err != nil {
		return nil, "", err
	}
	items = s.hydrate(ctx, items)

	next := ""
	if hasMore && len(items) > 0 {
		next = strconv.FormatInt(items[0].SequenceNo, 10)
	}
	return items, next, nil
}
