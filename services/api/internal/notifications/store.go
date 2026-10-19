package notifications

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// InsertParams is one notification to store. Only UserID and Type are required;
// the rest link the notification to whatever it is about.
type InsertParams struct {
	UserID         uuid.UUID
	Type           string
	ActorID        *uuid.UUID
	ConversationID *uuid.UUID
	MessageID      *uuid.UUID
	MomentID       *uuid.UUID
	CallID         *uuid.UUID
	Data           map[string]any
}

const notificationColumns = `n.id, n.user_id, n.type, n.actor_id,
	n.conversation_id, n.message_id, n.moment_id, n.call_id, n.data,
	n.read_at, n.created_at, a.display_name, a.username, a.avatar_url`

// Store provides notification persistence.
type Store struct {
	db *database.DB
}

// NewStore builds a notifications Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

func encodeData(data map[string]any) []byte {
	if len(data) == 0 {
		return []byte("{}")
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

// Insert stores one notification and returns it joined with its actor's
// profile, so the caller can publish the exact payload the client will render.
func (st *Store) Insert(ctx context.Context, p InsertParams) (Item, error) {
	row := st.db.Pool.QueryRow(ctx, `
		WITH n AS (
			INSERT INTO notifications
				(user_id, type, actor_id, conversation_id, message_id, moment_id, call_id, data)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, user_id, type, actor_id, conversation_id, message_id,
			          moment_id, call_id, data, read_at, created_at
		)
		SELECT `+notificationColumns+`
		FROM n
		LEFT JOIN users a ON a.id = n.actor_id`,
		p.UserID, p.Type, p.ActorID, p.ConversationID, p.MessageID, p.MomentID,
		p.CallID, encodeData(p.Data))
	item, err := scanItem(row)
	if err != nil {
		return Item{}, fmt.Errorf("insert notification: %w", err)
	}
	return item, nil
}

// List returns one page of a user's notifications, newest first, each joined
// with the actor's public profile.
func (st *Store) List(ctx context.Context, user uuid.UUID, after *Cursor, limit int) ([]Item, error) {
	var cursorAt any
	var cursorID any
	if after != nil {
		cursorAt = after.At
		cursorID = after.ID
	}

	rows, err := st.db.Pool.Query(ctx, `
		SELECT `+notificationColumns+`
		FROM notifications n
		LEFT JOIN users a ON a.id = n.actor_id
		WHERE n.user_id = $1
		  AND ($2::timestamptz IS NULL
		       OR (n.created_at, n.id) < ($2::timestamptz, $3::uuid))
		ORDER BY n.created_at DESC, n.id DESC
		LIMIT $4`,
		user, cursorAt, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanItem(row pgx.Row) (Item, error) {
	var (
		item        Item
		displayName *string
		username    *string
		avatarURL   *string
	)
	err := row.Scan(&item.ID, &item.UserID, &item.Type, &item.ActorID,
		&item.ConversationID, &item.MessageID, &item.MomentID, &item.CallID,
		&item.Data, &item.ReadAt, &item.CreatedAt,
		&displayName, &username, &avatarURL)
	if err != nil {
		return Item{}, err
	}
	// An actor row exists when either the id or its joined profile is present.
	if item.ActorID != nil {
		brief := &ActorBrief{ID: *item.ActorID}
		if displayName != nil {
			brief.DisplayName = *displayName
		}
		brief.Username = username
		brief.AvatarURL = avatarURL
		item.Actor = brief
	}
	return item, nil
}

// UnreadCount returns how many of a user's notifications are unread.
func (st *Store) UnreadCount(ctx context.Context, user uuid.UUID) (int64, error) {
	var count int64
	err := st.db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE user_id = $1 AND read_at IS NULL`, user).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return count, nil
}

// MarkRead marks the given notifications read, or every unread one when ids is
// empty. It returns the caller's remaining unread count.
func (st *Store) MarkRead(ctx context.Context, user uuid.UUID, ids []uuid.UUID) (int64, error) {
	if _, err := st.db.Pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1 AND read_at IS NULL
		  AND (cardinality($2::uuid[]) = 0 OR id = ANY($2::uuid[]))`,
		user, ids); err != nil {
		return 0, fmt.Errorf("mark read: %w", err)
	}
	return st.UnreadCount(ctx, user)
}

// Delete removes one notification owned by the user.
func (st *Store) Delete(ctx context.Context, user, id uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		DELETE FROM notifications WHERE id = $1 AND user_id = $2`, id, user)
	if err != nil {
		return false, fmt.Errorf("delete notification: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteAll removes every notification owned by the user.
func (st *Store) DeleteAll(ctx context.Context, user uuid.UUID) error {
	if _, err := st.db.Pool.Exec(ctx,
		`DELETE FROM notifications WHERE user_id = $1`, user); err != nil {
		return fmt.Errorf("delete notifications: %w", err)
	}
	return nil
}
