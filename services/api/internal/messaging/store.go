package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// ConversationCursor pages the conversation list by activity (newest first).
// The activity timestamp and the conversation id together form a total order,
// so ties cannot drop or repeat a row across pages.
type ConversationCursor struct {
	At time.Time
	ID uuid.UUID
}

// String renders the cursor for the `next_cursor` envelope field.
func (c ConversationCursor) String() string {
	return strconv.FormatInt(c.At.UnixMicro(), 10) + "|" + c.ID.String()
}

// ParseConversationCursor parses a cursor produced by String.
func ParseConversationCursor(raw string) (ConversationCursor, error) {
	parts := strings.Split(raw, "|")
	if len(parts) != 2 {
		return ConversationCursor{}, fmt.Errorf("malformed cursor")
	}
	micro, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ConversationCursor{}, fmt.Errorf("malformed cursor time: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return ConversationCursor{}, fmt.Errorf("malformed cursor id: %w", err)
	}
	return ConversationCursor{At: time.UnixMicro(micro).UTC(), ID: id}, nil
}

const conversationColumns = `id, kind, created_by, direct_key, title, avatar_url,
	next_sequence, last_message_at, created_at`

func scanConversation(row pgx.Row) (Conversation, error) {
	var c Conversation
	err := row.Scan(&c.ID, &c.Kind, &c.CreatedBy, &c.DirectKey, &c.Title,
		&c.AvatarURL, &c.NextSequence, &c.LastMessageAt, &c.CreatedAt)
	return c, err
}

// directKey normalizes a user pair into the unique key stored in
// conversations.direct_key, so (a,b) and (b,a) map to one conversation.
func directKey(a, b uuid.UUID) string {
	x, y := a.String(), b.String()
	if x > y {
		x, y = y, x
	}
	return x + ":" + y
}

// Store provides conversation and message persistence.
type Store struct {
	db *database.DB
}

// NewStore builds a messaging Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// --- summary queries -------------------------------------------------------

// conversationSummarySelect returns one row per conversation the caller
// belongs to: the conversation, the counterpart's profile, the newest message,
// and the unread count. Arguments are:
//
//	$1 caller, $2 cursor time (nullable), $3 cursor id (nullable),
//	$4 limit, $5 conversation id filter (nullable)
const conversationSummarySelect = `
	SELECT c.id, c.kind, c.created_by, c.direct_key, c.title, c.avatar_url,
	       c.next_sequence, c.last_message_at, c.created_at,
	       cm.role,
	       (SELECT COUNT(*) FROM conversation_members mc
	         WHERE mc.conversation_id = c.id),
	       COALESCE(p.id, '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE(p.display_name, ''),
	       p.username, p.avatar_url, p.status_message, p.last_seen_at,
	       lm.id IS NOT NULL,
	       COALESCE(lm.id, '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE(lm.sender_id, '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE(lm.sequence_no, 0),
	       lm.client_msg_id,
	       COALESCE(lm.message_type, ''),
	       COALESCE(lm.content, '{}'::jsonb),
	       COALESCE(lm.created_at, c.created_at),
	       lm.deleted_at,
	       (SELECT COUNT(*) FROM messages um
	         WHERE um.conversation_id = c.id
	           AND um.sequence_no > cm.last_read_sequence
	           AND um.sender_id <> $1)
	FROM conversation_members cm
	JOIN conversations c ON c.id = cm.conversation_id
	LEFT JOIN LATERAL (
		SELECT u.id, u.display_name, u.username, u.avatar_url,
		       u.status_message, u.last_seen_at
		FROM conversation_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.conversation_id = c.id AND om.user_id <> $1
		ORDER BY om.joined_at
		LIMIT 1
	) p ON TRUE
	LEFT JOIN messages lm
	  ON lm.conversation_id = c.id AND lm.sequence_no = c.next_sequence
	WHERE cm.user_id = $1
	  AND ($5::uuid IS NULL OR c.id = $5::uuid)
	  AND ($2::timestamptz IS NULL
	       OR COALESCE(c.last_message_at, c.created_at) < $2::timestamptz
	       OR (COALESCE(c.last_message_at, c.created_at) = $2::timestamptz
	           AND c.id < $3::uuid))
	ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
	LIMIT $4`

func scanConversationSummary(row pgx.Row) (ConversationSummary, error) {
	var (
		s            ConversationSummary
		peerID       uuid.UUID
		peerName     string
		hasLast      bool
		lastID       uuid.UUID
		lastSenderID uuid.UUID
		lastSeq      int64
		lastMsgID    *string
		lastType     string
		lastContent  json.RawMessage
		lastCreated  time.Time
		lastDeleted  *time.Time
	)

	err := row.Scan(
		&s.ID, &s.Kind, &s.CreatedBy, &s.DirectKey, &s.Title, &s.AvatarURL,
		&s.NextSequence, &s.LastMessageAt, &s.CreatedAt,
		&s.Role, &s.MemberCount,
		&peerID, &peerName, &s.Peer.Username, &s.Peer.AvatarURL,
		&s.Peer.StatusMessage, &s.Peer.LastSeenAt,
		&hasLast, &lastID, &lastSenderID, &lastSeq, &lastMsgID,
		&lastType, &lastContent, &lastCreated, &lastDeleted,
		&s.Unread,
	)
	if err != nil {
		return ConversationSummary{}, err
	}

	s.Peer.ID = peerID
	s.Peer.DisplayName = peerName
	if peerID == uuid.Nil {
		s.Peer.DisplayName = "Unknown user"
	}

	if hasLast {
		s.LastMessage = &Message{
			ID:             lastID,
			ConversationID: s.ID,
			SenderID:       lastSenderID,
			SequenceNo:     lastSeq,
			ClientMsgID:    lastMsgID,
			MessageType:    lastType,
			Content:        lastContent,
			CreatedAt:      lastCreated,
			DeletedAt:      lastDeleted,
		}
	}
	return s, nil
}

// ListConversations returns the caller's conversations ordered by activity,
// newest first, together with the cursor for the next page.
func (st *Store) ListConversations(ctx context.Context, me uuid.UUID, after *ConversationCursor, limit int) ([]ConversationSummary, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}

	rows, err := st.db.Pool.Query(ctx, conversationSummarySelect,
		me, cursorAt, cursorID, limit, nil)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	out := make([]ConversationSummary, 0)
	for rows.Next() {
		s, err := scanConversationSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetConversationSummary loads one conversation the caller belongs to.
// ErrConversationNotFound covers both "does not exist" and "not a member" so
// the endpoint cannot be used to probe for other people's conversation ids.
func (st *Store) GetConversationSummary(ctx context.Context, me, id uuid.UUID) (ConversationSummary, error) {
	rows, err := st.db.Pool.Query(ctx, conversationSummarySelect, me, nil, nil, 1, id)
	if err != nil {
		return ConversationSummary{}, fmt.Errorf("get conversation: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return ConversationSummary{}, fmt.Errorf("get conversation: %w", err)
		}
		return ConversationSummary{}, ErrConversationNotFound
	}
	s, err := scanConversationSummary(rows)
	if err != nil {
		return ConversationSummary{}, fmt.Errorf("scan conversation: %w", err)
	}
	return s, nil
}

// --- membership -------------------------------------------------------------

// IsMember reports whether the user belongs to the conversation.
func (st *Store) IsMember(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := st.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM conversation_members
			WHERE conversation_id = $1 AND user_id = $2
		)`, conversationID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("is member: %w", err)
	}
	return ok, nil
}

// MemberIDs lists every member of a conversation.
func (st *Store) MemberIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := st.db.Pool.Query(ctx, `
		SELECT user_id FROM conversation_members
		WHERE conversation_id = $1
		ORDER BY joined_at`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("member ids: %w", err)
	}
	defer rows.Close()

	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MemberState returns the caller's acknowledged sequence positions.
func (st *Store) MemberState(ctx context.Context, conversationID, userID uuid.UUID) (MemberReadState, error) {
	var s MemberReadState
	err := st.db.Pool.QueryRow(ctx, `
		SELECT last_read_sequence, last_delivered_sequence
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&s.LastReadSequence, &s.LastDeliveredSequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberReadState{}, ErrNotMember
	}
	if err != nil {
		return MemberReadState{}, fmt.Errorf("member state: %w", err)
	}
	return s, nil
}

// Conversation loads a conversation row by id.
func (st *Store) Conversation(ctx context.Context, id uuid.UUID) (Conversation, error) {
	c, err := scanConversation(st.db.Pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("conversation: %w", err)
	}
	return c, nil
}

// --- creation ---------------------------------------------------------------

// FindOrCreateDirect returns the direct conversation between a and b,
// creating it (and both membership rows) when absent. It reports whether the
// conversation was created by this call. The unique partial index on
// direct_key makes concurrent creators converge on one row.
func (st *Store) FindOrCreateDirect(ctx context.Context, a, b uuid.UUID) (Conversation, bool, error) {
	key := directKey(a, b)

	if conv, found, err := st.findByDirectKey(ctx, key); err != nil || found {
		return conv, false, err
	}

	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return Conversation{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	conv, err := scanConversation(tx.QueryRow(ctx, `
		INSERT INTO conversations (kind, created_by, direct_key)
		VALUES ('direct', $1, $2)
		ON CONFLICT (direct_key) WHERE direct_key IS NOT NULL DO NOTHING
		RETURNING `+conversationColumns, a, key))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Lost the race: another request created it first.
		if err := tx.Rollback(ctx); err != nil {
			return Conversation{}, false, fmt.Errorf("rollback: %w", err)
		}
		existing, found, ferr := st.findByDirectKey(ctx, key)
		if ferr != nil {
			return Conversation{}, false, ferr
		}
		if !found {
			return Conversation{}, false, ErrConversationNotFound
		}
		return existing, false, nil
	case err != nil:
		return Conversation{}, false, fmt.Errorf("create conversation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id)
		VALUES ($1, $2), ($1, $3)
		ON CONFLICT DO NOTHING`, conv.ID, a, b); err != nil {
		return Conversation{}, false, fmt.Errorf("insert members: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, false, fmt.Errorf("commit tx: %w", err)
	}
	return conv, true, nil
}

func (st *Store) findByDirectKey(ctx context.Context, key string) (Conversation, bool, error) {
	conv, err := scanConversation(st.db.Pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE direct_key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, false, nil
	}
	if err != nil {
		return Conversation{}, false, fmt.Errorf("find conversation: %w", err)
	}
	return conv, true, nil
}

// UserExists reports whether a user id is present in the users table.
func (st *Store) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := st.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("user exists: %w", err)
	}
	return ok, nil
}

// CanMessage reports whether a and b may open a direct conversation: they must
// be connected in both directions and neither may have blocked the other.
func (st *Store) CanMessage(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var ok bool
	err := st.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM contacts
			WHERE status = 'accepted'
			  AND ((user_id = $1 AND contact_id = $2)
			    OR (user_id = $2 AND contact_id = $1))
		) AND NOT EXISTS(
			SELECT 1 FROM contacts
			WHERE status = 'blocked'
			  AND ((user_id = $1 AND contact_id = $2)
			    OR (user_id = $2 AND contact_id = $1))
		)`, a, b).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("can message: %w", err)
	}
	return ok, nil
}

// --- messages ---------------------------------------------------------------

// InsertMessageParams carries a new message's inputs.
type InsertMessageParams struct {
	ConversationID uuid.UUID
	SenderID       uuid.UUID
	ClientMsgID    *string
	MessageType    string
	Content        json.RawMessage
}

const messageColumns = `id, conversation_id, sender_id, sequence_no,
	client_msg_id, message_type, content, created_at, edited_at, deleted_at`

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.SequenceNo,
		&m.ClientMsgID, &m.MessageType, &m.Content, &m.CreatedAt,
		&m.EditedAt, &m.DeletedAt)
	return m, err
}

// InsertMessage persists a message inside a transaction that also allocates its
// conversation sequence, so concurrent senders serialize on the conversation
// row and the sequence stays gap-free and monotonic. Replaying a previously
// used client_msg_id returns the original row with duplicate=true instead of
// inserting a second copy; the whole transaction is rolled back in that case so
// no sequence is consumed.
func (st *Store) InsertMessage(ctx context.Context, p InsertMessageParams) (Message, bool, error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return Message{}, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if p.ClientMsgID != nil {
		existing, found, err := findMessageByClientID(ctx, tx, p.SenderID, *p.ClientMsgID)
		if err != nil {
			return Message{}, false, err
		}
		if found {
			return existing, true, nil
		}
	}

	var seq int64
	err = tx.QueryRow(ctx, `
		UPDATE conversations
		SET next_sequence = next_sequence + 1, last_message_at = now()
		WHERE id = $1
		RETURNING next_sequence`, p.ConversationID).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, ErrConversationNotFound
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("allocate sequence: %w", err)
	}

	msg, err := scanMessage(tx.QueryRow(ctx, `
		INSERT INTO messages
			(conversation_id, sender_id, sequence_no, client_msg_id, message_type, content)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (sender_id, client_msg_id) WHERE client_msg_id IS NOT NULL
		DO NOTHING
		RETURNING `+messageColumns,
		p.ConversationID, p.SenderID, seq, p.ClientMsgID, p.MessageType, p.Content))
	if errors.Is(err, pgx.ErrNoRows) {
		// Concurrent retry with the same idempotency key won. The caller's
		// sequence allocation is discarded with the transaction.
		if p.ClientMsgID == nil {
			return Message{}, false, fmt.Errorf("insert message: conflict without client id")
		}
		existing, found, ferr := findMessageByClientID(ctx, tx, p.SenderID, *p.ClientMsgID)
		if ferr != nil {
			return Message{}, false, ferr
		}
		if !found {
			return Message{}, false, fmt.Errorf("insert message: conflict did not resolve")
		}
		return existing, true, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("insert message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Message{}, false, fmt.Errorf("commit tx: %w", err)
	}
	return msg, false, nil
}

func findMessageByClientID(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, senderID uuid.UUID, clientMsgID string) (Message, bool, error) {
	m, err := scanMessage(q.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE sender_id = $1 AND client_msg_id = $2`,
		senderID, clientMsgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("find message by client id: %w", err)
	}
	return m, true, nil
}

// MessageQuery scopes a page of history. Exactly one of Before/After is used
// by a given request; Before walks backwards through history and After is the
// reconnect catch-up path.
type MessageQuery struct {
	Limit  int
	Before *int64
	After  *int64
}

// ListMessages returns messages for a conversation the caller belongs to, in
// ascending sequence order, plus the cursor for loading older history.
//
// Arguments: $1 conversation, $2 viewer, $3 before, $4 after, $5 limit.
const messageListSelect = `
	SELECT m.id, m.conversation_id, m.sender_id, m.sequence_no, m.client_msg_id,
	       m.message_type, m.content, m.created_at, m.edited_at, m.deleted_at,
	       CASE
	         WHEN m.sender_id = $2 THEN
	           CASE
	             WHEN COALESCE(peer.last_read_sequence, 0) >= m.sequence_no THEN '` + StatusRead + `'
	             WHEN COALESCE(peer.last_delivered_sequence, 0) >= m.sequence_no THEN '` + StatusDelivered + `'
	             ELSE '` + StatusSent + `'
	           END
	         ELSE
	           CASE
	             WHEN me.last_read_sequence >= m.sequence_no THEN '` + StatusRead + `'
	             ELSE '` + StatusReceived + `'
	           END
	       END
	FROM messages m
	JOIN conversation_members me
	  ON me.conversation_id = m.conversation_id AND me.user_id = $2
	LEFT JOIN LATERAL (
		SELECT om.last_read_sequence, om.last_delivered_sequence
		FROM conversation_members om
		WHERE om.conversation_id = m.conversation_id AND om.user_id <> $2
		LIMIT 1
	) peer ON TRUE
	WHERE m.conversation_id = $1
	  AND ($3::bigint IS NULL OR m.sequence_no < $3::bigint)
	  AND ($4::bigint IS NULL OR m.sequence_no > $4::bigint)
	ORDER BY m.sequence_no DESC
	LIMIT $5`

func scanMessageRow(row pgx.Row) (Message, string, error) {
	var (
		m      Message
		status string
	)
	err := row.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.SequenceNo,
		&m.ClientMsgID, &m.MessageType, &m.Content, &m.CreatedAt,
		&m.EditedAt, &m.DeletedAt, &status)
	if err != nil {
		return Message{}, "", err
	}
	m.Status = status
	return m, status, nil
}

// ListMessages returns one page of history oldest-first and reports whether
// older history remains (so the caller can hand back a `before` cursor).
func (st *Store) ListMessages(ctx context.Context, conversationID, viewer uuid.UUID, q MessageQuery) ([]Message, bool, error) {
	rows, err := st.db.Pool.Query(ctx, messageListSelect,
		conversationID, viewer, q.Before, q.After, q.Limit)
	if err != nil {
		return nil, false, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	// The query walks backwards; flip it so clients render top-down.
	desc := make([]Message, 0, q.Limit)
	for rows.Next() {
		m, _, err := scanMessageRow(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan message: %w", err)
		}
		desc = append(desc, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate messages: %w", err)
	}

	hasMore := len(desc) == q.Limit
	out := make([]Message, 0, len(desc))
	for i := len(desc) - 1; i >= 0; i-- {
		out = append(out, desc[i])
	}
	return out, hasMore, nil
}

// StatusFor computes the viewer-relative status of one message.
func (st *Store) StatusFor(ctx context.Context, conversationID, viewer, sender uuid.UUID, sequence int64) (string, error) {
	var status string
	err := st.db.Pool.QueryRow(ctx, `
		SELECT CASE
		         WHEN $2::uuid = $3::uuid THEN
		           CASE
		             WHEN COALESCE(peer.last_read_sequence, 0) >= $4 THEN '`+StatusRead+`'
		             WHEN COALESCE(peer.last_delivered_sequence, 0) >= $4 THEN '`+StatusDelivered+`'
		             ELSE '`+StatusSent+`'
		           END
		         ELSE
		           CASE
		             WHEN me.last_read_sequence >= $4 THEN '`+StatusRead+`'
		             ELSE '`+StatusReceived+`'
		           END
		       END
		FROM conversation_members me
		LEFT JOIN LATERAL (
			SELECT om.last_read_sequence, om.last_delivered_sequence
			FROM conversation_members om
			WHERE om.conversation_id = me.conversation_id AND om.user_id <> $2
			LIMIT 1
		) peer ON TRUE
		WHERE me.conversation_id = $1 AND me.user_id = $2`,
		conversationID, viewer, sender, sequence).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("status for: %w", err)
	}
	return status, nil
}

// AdvanceRead moves the member's read position forward and returns the new
// position. It is idempotent: an older sequence never rewinds it.
func (st *Store) AdvanceRead(ctx context.Context, conversationID, userID uuid.UUID, upTo int64) (int64, error) {
	var seq int64
	err := st.db.Pool.QueryRow(ctx, `
		UPDATE conversation_members
		SET last_read_sequence = GREATEST(last_read_sequence, $3),
		    last_delivered_sequence = GREATEST(last_delivered_sequence, $3)
		WHERE conversation_id = $1 AND user_id = $2
		RETURNING last_read_sequence`,
		conversationID, userID, upTo).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotMember
	}
	if err != nil {
		return 0, fmt.Errorf("advance read: %w", err)
	}
	return seq, nil
}

// AdvanceDelivered moves the member's delivered position forward and reports
// the new position, or ErrConversationNotFound when nothing moved (which is
// the common case: the position is already current).
func (st *Store) AdvanceDelivered(ctx context.Context, conversationID, userID uuid.UUID, upTo int64) (int64, error) {
	var seq int64
	err := st.db.Pool.QueryRow(ctx, `
		UPDATE conversation_members
		SET last_delivered_sequence = GREATEST(last_delivered_sequence, $3)
		WHERE conversation_id = $1 AND user_id = $2 AND last_delivered_sequence < $3
		RETURNING last_delivered_sequence`,
		conversationID, userID, upTo).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConversationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("advance delivered: %w", err)
	}
	return seq, nil
}

// AdvanceDeliveredForUser marks every conversation's history as delivered to a
// member who just (re)connected, and returns the conversations whose position
// actually moved so receipts can be published to the senders.
func (st *Store) AdvanceDeliveredForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := st.db.Pool.Query(ctx, `
		UPDATE conversation_members cm
		SET last_delivered_sequence = c.next_sequence
		FROM conversations c
		WHERE c.id = cm.conversation_id
		  AND cm.user_id = $1
		  AND cm.last_delivered_sequence < c.next_sequence
		RETURNING cm.conversation_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("advance delivered for user: %w", err)
	}
	defer rows.Close()

	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UnreadCount returns how many messages sent by others are past the given read
// position.
func (st *Store) UnreadCount(ctx context.Context, conversationID, reader uuid.UUID, readSequence int64) (int64, error) {
	var n int64
	err := st.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM messages
		WHERE conversation_id = $1
		  AND sequence_no > $2
		  AND sender_id <> $3`,
		conversationID, readSequence, reader).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return n, nil
}

// PeekSequence returns a conversation's current next_sequence without
// allocating one, used to bound validation of read positions.
func (st *Store) PeekSequence(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var seq int64
	err := st.db.Pool.QueryRow(ctx,
		`SELECT next_sequence FROM conversations WHERE id = $1`,
		conversationID).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConversationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("peek sequence: %w", err)
	}
	return seq, nil
}

// --- groups -----------------------------------------------------------------

// MemberRole returns a member's role in a conversation, or ErrNotMember.
func (st *Store) MemberRole(ctx context.Context, conversationID, userID uuid.UUID) (string, error) {
	var role string
	err := st.db.Pool.QueryRow(ctx, `
		SELECT role FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", fmt.Errorf("member role: %w", err)
	}
	return role, nil
}

// CountMembers returns how many members a conversation has.
func (st *Store) CountMembers(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var n int64
	err := st.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM conversation_members WHERE conversation_id = $1`,
		conversationID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count members: %w", err)
	}
	return n, nil
}

// MemberDetails lists a conversation's members joined with their public
// profiles, oldest join first.
func (st *Store) MemberDetails(ctx context.Context, conversationID uuid.UUID) ([]MemberDetail, error) {
	rows, err := st.db.Pool.Query(ctx, `
		SELECT cm.user_id, cm.role, u.display_name, u.username, u.avatar_url,
		       u.status_message, u.last_seen_at, cm.joined_at
		FROM conversation_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.conversation_id = $1
		ORDER BY cm.joined_at, cm.user_id`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("member details: %w", err)
	}
	defer rows.Close()

	out := make([]MemberDetail, 0)
	for rows.Next() {
		var m MemberDetail
		if err := rows.Scan(&m.UserID, &m.Role, &m.DisplayName, &m.Username,
			&m.AvatarURL, &m.StatusMessage, &m.LastSeenAt, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ProfileBrief loads the public profile fields for one user, used to render
// system-message text with display names.
func (st *Store) ProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error) {
	var p ProfileBrief
	err := st.db.Pool.QueryRow(ctx, `
		SELECT id, display_name, username, avatar_url, status_message, last_seen_at
		FROM users WHERE id = $1`, id,
	).Scan(&p.ID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.StatusMessage, &p.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProfileBrief{}, ErrPeerNotFound
	}
	if err != nil {
		return ProfileBrief{}, fmt.Errorf("profile brief: %w", err)
	}
	return p, nil
}

// CreateGroup creates a group conversation owned by `owner` with the given
// initial members (owner included implicitly). Member rows are inserted in one
// statement so the group never exists without its owner.
func (st *Store) CreateGroup(ctx context.Context, owner uuid.UUID, title string, memberIDs []uuid.UUID) (Conversation, error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return Conversation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	conv, err := scanConversation(tx.QueryRow(ctx, `
		INSERT INTO conversations (kind, created_by, title)
		VALUES ('group', $1, $2)
		RETURNING `+conversationColumns, owner, title))
	if err != nil {
		return Conversation{}, fmt.Errorf("create group: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id, role)
		VALUES ($1, $2, '`+RoleOwner+`')`, conv.ID, owner); err != nil {
		return Conversation{}, fmt.Errorf("insert owner: %w", err)
	}

	others := make([]uuid.UUID, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id != owner {
			others = append(others, id)
		}
	}
	if len(others) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_members (conversation_id, user_id, role)
			SELECT $1, unnest($2::uuid[]), '`+RoleMember+`'
			ON CONFLICT DO NOTHING`, conv.ID, others); err != nil {
			return Conversation{}, fmt.Errorf("insert members: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, fmt.Errorf("commit tx: %w", err)
	}
	return conv, nil
}

// UpdateGroupMeta sets a group's title and/or avatar. Nil fields are left
// unchanged; empty-string title clears the title.
func (st *Store) UpdateGroupMeta(ctx context.Context, id uuid.UUID, title, avatarURL *string) (Conversation, error) {
	conv, err := scanConversation(st.db.Pool.QueryRow(ctx, `
		UPDATE conversations
		SET title = COALESCE($2, title),
		    avatar_url = COALESCE($3, avatar_url)
		WHERE id = $1 AND kind = 'group'
		RETURNING `+conversationColumns, id, title, avatarURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("update group: %w", err)
	}
	return conv, nil
}

// AddMembers adds users to a group, returning how many rows were new. Existing
// members are ignored.
func (st *Store) AddMembers(ctx context.Context, id uuid.UUID, userIDs []uuid.UUID) (int64, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id, role)
		SELECT $1, unnest($2::uuid[]), '`+RoleMember+`'
		ON CONFLICT DO NOTHING`, id, userIDs)
	if err != nil {
		return 0, fmt.Errorf("add members: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RemoveMember deletes a membership row, reporting whether it existed.
func (st *Store) RemoveMember(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		DELETE FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, fmt.Errorf("remove member: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetMemberRole updates a non-owner member's role, reporting whether it changed.
func (st *Store) SetMemberRole(ctx context.Context, id, userID uuid.UUID, role string) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE conversation_members SET role = $3
		WHERE conversation_id = $1 AND user_id = $2 AND role <> '`+RoleOwner+`'`,
		id, userID, role)
	if err != nil {
		return false, fmt.Errorf("set member role: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// LeaveGroup removes the caller from a group. When the caller owns it, the
// earliest-joined remaining member is promoted to owner; when the caller is the
// last member the conversation (and its history) is deleted. It reports whether
// the conversation was deleted and, when ownership moved, the new owner.
func (st *Store) LeaveGroup(ctx context.Context, id, userID uuid.UUID) (deleted bool, newOwner *uuid.UUID, err error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role string
	err = tx.QueryRow(ctx, `
		SELECT role FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2 FOR UPDATE`,
		id, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, ErrNotMember
	}
	if err != nil {
		return false, nil, fmt.Errorf("lock membership: %w", err)
	}

	if role == RoleOwner {
		var heir uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT user_id FROM conversation_members
			WHERE conversation_id = $1 AND user_id <> $2
			ORDER BY joined_at, user_id
			LIMIT 1 FOR UPDATE`, id, userID).Scan(&heir)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Last member: drop the conversation entirely.
			if _, err := tx.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id); err != nil {
				return false, nil, fmt.Errorf("delete conversation: %w", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return false, nil, fmt.Errorf("commit tx: %w", err)
			}
			return true, nil, nil
		case err != nil:
			return false, nil, fmt.Errorf("find heir: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversation_members SET role = '`+RoleOwner+`'
			WHERE conversation_id = $1 AND user_id = $2`, id, heir); err != nil {
			return false, nil, fmt.Errorf("promote heir: %w", err)
		}
		newOwner = &heir
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2`, id, userID); err != nil {
		return false, nil, fmt.Errorf("leave: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, nil, fmt.Errorf("commit tx: %w", err)
	}
	return false, newOwner, nil
}
