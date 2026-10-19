package search

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/database"
	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// Store reads search results from Postgres.
type Store struct {
	db *database.DB
}

// NewStore builds a search Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// SearchMessages returns one page of the caller's own text messages matching
// query, newest first. Membership is enforced in the query, so a message the
// caller cannot see is never returned.
func (st *Store) SearchMessages(ctx context.Context, me uuid.UUID, query string, after *Cursor, limit int) ([]MessageHit, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}

	rows, err := st.db.Pool.Query(ctx, `
		SELECT m.id, m.conversation_id, m.sender_id, m.sequence_no,
		       m.client_msg_id, m.message_type, m.content, m.created_at,
		       m.edited_at, m.deleted_at,
		       c.id, c.kind, c.title,
		       p.id, p.username, p.display_name, p.avatar_url
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN conversation_members mine
		     ON mine.conversation_id = m.conversation_id AND mine.user_id = $1
		LEFT JOIN LATERAL (
			SELECT u.id, u.username, u.display_name, u.avatar_url
			FROM conversation_members cm
			JOIN users u ON u.id = cm.user_id
			WHERE cm.conversation_id = m.conversation_id AND cm.user_id <> $1
			ORDER BY cm.joined_at
			LIMIT 1
		) p ON c.kind = 'direct'
		WHERE m.deleted_at IS NULL
		  AND m.message_type = 'text'
		  AND m.content->>'text' ILIKE '%' || $2 || '%' ESCAPE '\'
		  AND ($3::timestamptz IS NULL
		       OR (m.created_at, m.id) < ($3::timestamptz, $4::uuid))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $5`,
		me, query, cursorAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	out := make([]MessageHit, 0)
	for rows.Next() {
		var (
			hit      MessageHit
			convID   uuid.UUID
			kind     string
			title    *string
			peerID   *uuid.UUID
			username *string
			display  *string
			avatar   *string
		)
		if err := rows.Scan(
			&hit.Message.ID, &hit.Message.ConversationID, &hit.Message.SenderID,
			&hit.Message.SequenceNo, &hit.Message.ClientMsgID,
			&hit.Message.MessageType, &hit.Message.Content, &hit.Message.CreatedAt,
			&hit.Message.EditedAt, &hit.Message.DeletedAt,
			&convID, &kind, &title,
			&peerID, &username, &display, &avatar,
		); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		// Search results are snippets; delivery state is not meaningful here,
		// so report the viewer-relative baseline (received / sent).
		if hit.Message.SenderID == me {
			hit.Message.Status = messaging.StatusSent
		} else {
			hit.Message.Status = messaging.StatusReceived
		}
		hit.Conversation = ConversationContext{ID: convID, Kind: kind, Title: title}
		if kind == "direct" && peerID != nil {
			peer := &Peer{ID: *peerID, Username: username, AvatarURL: avatar}
			if display != nil {
				peer.DisplayName = *display
			}
			hit.Conversation.Peer = peer
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

// SearchProfiles returns one page of public profiles matching query by username
// or display name, excluding the caller and anyone on either side of a block.
// Results are ordered newest-account-first with a (created_at, id) keyset.
func (st *Store) SearchProfiles(ctx context.Context, me uuid.UUID, query string, after *Cursor, limit int) ([]ProfileHit, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}
	rows, err := st.db.Pool.Query(ctx, `
		SELECT u.id, u.username, u.display_name, u.avatar_url, u.status_message,
		       u.last_seen_at, u.created_at,
		       EXISTS (
		         SELECT 1 FROM contacts c
		         WHERE c.status = 'accepted'
		           AND ((c.user_id = $1 AND c.contact_id = u.id)
		             OR (c.user_id = u.id AND c.contact_id = $1))
		       ) AS is_contact
		FROM users u
		WHERE u.id <> $1
		  AND NOT EXISTS (
		    SELECT 1 FROM contacts b
		    WHERE b.status = 'blocked'
		      AND ((b.user_id = $1 AND b.contact_id = u.id)
		        OR (b.user_id = u.id AND b.contact_id = $1))
		  )
		  AND (u.username ILIKE '%' || $2 || '%' ESCAPE '\'
		       OR u.display_name ILIKE '%' || $2 || '%' ESCAPE '\')
		  AND ($3::timestamptz IS NULL
		       OR (u.created_at, u.id) < ($3::timestamptz, $4::uuid))
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT $5`,
		me, query, cursorAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("search profiles: %w", err)
	}
	defer rows.Close()

	out := make([]ProfileHit, 0)
	for rows.Next() {
		var hit ProfileHit
		if err := rows.Scan(&hit.ID, &hit.Username, &hit.DisplayName,
			&hit.AvatarURL, &hit.StatusMessage, &hit.LastSeenAt, &hit.CreatedAt,
			&hit.IsContact); err != nil {
			return nil, fmt.Errorf("scan profile hit: %w", err)
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

// DiscoverProfiles returns one page of people the caller is not already
// connected to and has not blocked, newest-account-first. This backs the
// "people you may know" rail on the Discover page.
func (st *Store) DiscoverProfiles(ctx context.Context, me uuid.UUID, after *Cursor, limit int) ([]ProfileHit, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}
	rows, err := st.db.Pool.Query(ctx, `
		SELECT u.id, u.username, u.display_name, u.avatar_url, u.status_message,
		       u.last_seen_at, u.created_at
		FROM users u
		WHERE u.id <> $1
		  AND NOT EXISTS (
		    SELECT 1 FROM contacts b
		    WHERE b.status = 'blocked'
		      AND ((b.user_id = $1 AND b.contact_id = u.id)
		        OR (b.user_id = u.id AND b.contact_id = $1))
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM contacts f
		    WHERE f.status = 'accepted'
		      AND ((f.user_id = $1 AND f.contact_id = u.id)
		        OR (f.user_id = u.id AND f.contact_id = $1))
		  )
		  AND ($2::timestamptz IS NULL
		       OR (u.created_at, u.id) < ($2::timestamptz, $3::uuid))
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT $4`,
		me, cursorAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("discover profiles: %w", err)
	}
	defer rows.Close()

	out := make([]ProfileHit, 0)
	for rows.Next() {
		var hit ProfileHit
		// is_contact is false by construction (accepted contacts are excluded).
		if err := rows.Scan(&hit.ID, &hit.Username, &hit.DisplayName,
			&hit.AvatarURL, &hit.StatusMessage, &hit.LastSeenAt,
			&hit.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan discover profile: %w", err)
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

// SearchConversations returns one page of the caller's conversations whose
// group title (for groups) or peer name (for direct chats) matches query,
// ordered by recent activity. `kind` optionally restricts results to "direct"
// or "group"; an empty kind matches both. Membership is enforced in SQL.
func (st *Store) SearchConversations(ctx context.Context, me uuid.UUID, query, kind string, after *Cursor, limit int) ([]ConversationHit, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}
	var kindArg any
	if kind != "" {
		kindArg = kind
	}
	rows, err := st.db.Pool.Query(ctx, `
		SELECT c.id, c.kind, c.title, c.created_at, c.last_message_at, cm.role,
		       (SELECT count(*) FROM conversation_members mc
		         WHERE mc.conversation_id = c.id) AS member_count,
		       p.id, p.username, p.display_name, p.avatar_url
		FROM conversation_members cm
		JOIN conversations c ON c.id = cm.conversation_id
		LEFT JOIN LATERAL (
			SELECT u.id, u.username, u.display_name, u.avatar_url
			FROM conversation_members om
			JOIN users u ON u.id = om.user_id
			WHERE om.conversation_id = c.id AND om.user_id <> $1
			ORDER BY om.joined_at
			LIMIT 1
		) p ON c.kind = 'direct'
		WHERE cm.user_id = $1
		  AND ($2::text IS NULL OR c.kind = $2::text)
		  AND (
		    (c.kind = 'group' AND c.title ILIKE '%' || $3 || '%' ESCAPE '\')
		    OR (c.kind = 'direct' AND p.display_name ILIKE '%' || $3 || '%' ESCAPE '\')
		    OR (c.kind = 'direct' AND p.username ILIKE '%' || $3 || '%' ESCAPE '\')
		  )
		  AND ($4::timestamptz IS NULL
		       OR (COALESCE(c.last_message_at, c.created_at), c.id) < ($4::timestamptz, $5::uuid))
		ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
		LIMIT $6`,
		me, kindArg, query, cursorAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("search conversations: %w", err)
	}
	defer rows.Close()

	out := make([]ConversationHit, 0)
	for rows.Next() {
		var (
			hit      ConversationHit
			peerID   *uuid.UUID
			username *string
			display  *string
			avatar   *string
		)
		if err := rows.Scan(&hit.ID, &hit.Kind, &hit.Title, &hit.CreatedAt,
			&hit.LastMessageAt, &hit.Role, &hit.MemberCount,
			&peerID, &username, &display, &avatar); err != nil {
			return nil, fmt.Errorf("scan conversation hit: %w", err)
		}
		if hit.Kind == "direct" && peerID != nil {
			peer := &Peer{ID: *peerID, Username: username, AvatarURL: avatar}
			if display != nil {
				peer.DisplayName = *display
			}
			hit.Peer = peer
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}
