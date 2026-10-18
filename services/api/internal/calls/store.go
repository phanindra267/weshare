package calls

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// Store provides call-history persistence on top of PostgreSQL. It also owns
// the two authorization/identity lookups the service needs, so the calls
// package stays independent of the contacts package.
type Store struct {
	db *database.DB
}

// NewStore builds a calls Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// AreConnected reports whether two users are accepted contacts with no block in
// either direction. It is the single authorization gate for placing a call.
func (st *Store) AreConnected(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var ok bool
	err := st.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM contacts
			WHERE user_id = $1 AND contact_id = $2 AND status = 'accepted'
		) AND NOT EXISTS(
			SELECT 1 FROM contacts
			WHERE status = 'blocked'
			  AND ((user_id = $1 AND contact_id = $2)
			    OR (user_id = $2 AND contact_id = $1))
		)`, a, b).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("are connected: %w", err)
	}
	return ok, nil
}

// ProfileBrief loads the public profile fields for one user.
func (st *Store) ProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error) {
	var p ProfileBrief
	err := st.db.Pool.QueryRow(ctx, `
		SELECT id, display_name, username, avatar_url
		FROM users WHERE id = $1`, id,
	).Scan(&p.ID, &p.DisplayName, &p.Username, &p.AvatarURL)
	if err != nil {
		return ProfileBrief{}, fmt.Errorf("profile brief: %w", err)
	}
	return p, nil
}

// CreateCall inserts the call summary (status `ringing`) and its participants:
// the caller is recorded as joined, each callee as invited. It runs in one
// transaction so a call never exists without its participants.
func (st *Store) CreateCall(ctx context.Context, id, caller uuid.UUID, kind string, isGroup bool, callees []uuid.UUID) error {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin call: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO calls (id, caller_id, kind, is_group, status)
		VALUES ($1, $2, $3, $4, '`+StatusRinging+`')`,
		id, caller, kind, isGroup); err != nil {
		return fmt.Errorf("insert call: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO call_participants (call_id, user_id, role, state, joined_at)
		VALUES ($1, $2, '`+RoleCaller+`', '`+ParticipantJoined+`', now())`,
		id, caller); err != nil {
		return fmt.Errorf("insert caller participant: %w", err)
	}

	for _, callee := range callees {
		if _, err := tx.Exec(ctx, `
			INSERT INTO call_participants (call_id, user_id, role, state)
			VALUES ($1, $2, '`+RoleCallee+`', '`+ParticipantInvited+`')`,
			id, callee); err != nil {
			return fmt.Errorf("insert callee participant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit call: %w", err)
	}
	return nil
}

// MarkCallActive records the moment the first callee joined a ringing call.
func (st *Store) MarkCallActive(ctx context.Context, id uuid.UUID) error {
	if _, err := st.db.Pool.Exec(ctx, `
		UPDATE calls SET status = '`+StatusActive+`', started_at = now()
		WHERE id = $1 AND status = '`+StatusRinging+`'`, id); err != nil {
		return fmt.Errorf("mark call active: %w", err)
	}
	return nil
}

// FinishCall sets a terminal status and end time on a call that has not already
// finished. It is idempotent.
func (st *Store) FinishCall(ctx context.Context, id uuid.UUID, status string) error {
	if _, err := st.db.Pool.Exec(ctx, `
		UPDATE calls SET status = $2, ended_at = now()
		WHERE id = $1 AND status IN ('`+StatusRinging+`', '`+StatusActive+`')`,
		id, status); err != nil {
		return fmt.Errorf("finish call: %w", err)
	}
	return nil
}

// SetParticipantState updates one participant's state within a call.
func (st *Store) SetParticipantState(ctx context.Context, callID, userID uuid.UUID, state string) error {
	if _, err := st.db.Pool.Exec(ctx, `
		UPDATE call_participants
		SET state = $3,
		    joined_at = CASE WHEN $3 = '`+ParticipantJoined+`' AND joined_at IS NULL THEN now() ELSE joined_at END,
		    left_at = CASE WHEN $3 IN ('`+ParticipantLeft+`', '`+ParticipantRejected+`', '`+ParticipantMissed+`') THEN now() ELSE left_at END
		WHERE call_id = $1 AND user_id = $2`,
		callID, userID, state); err != nil {
		return fmt.Errorf("set participant state: %w", err)
	}
	return nil
}

// ListCalls returns the most recent calls a user took part in, newest first,
// with participants (and their profiles) attached.
func (st *Store) ListCalls(ctx context.Context, user uuid.UUID, limit int) ([]Call, error) {
	rows, err := st.db.Pool.Query(ctx, `
		SELECT c.id, c.caller_id, c.kind, c.is_group, c.status,
		       c.started_at, c.ended_at, c.created_at
		FROM calls c
		WHERE EXISTS (SELECT 1 FROM call_participants p
		               WHERE p.call_id = c.id AND p.user_id = $1)
		ORDER BY c.created_at DESC
		LIMIT $2`, user, limit)
	if err != nil {
		return nil, fmt.Errorf("list calls: %w", err)
	}
	defer rows.Close()

	callList := make([]Call, 0, limit)
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.ID, &c.CallerID, &c.Kind, &c.IsGroup, &c.Status,
			&c.StartedAt, &c.EndedAt, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan call: %w", err)
		}
		callList = append(callList, c)
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calls: %w", err)
	}
	if len(ids) == 0 {
		return callList, nil
	}

	participants, err := st.loadParticipants(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range callList {
		callList[i].Participants = participants[callList[i].ID]
	}
	return callList, nil
}

// loadParticipants fetches the participants (with profiles) for a set of calls.
func (st *Store) loadParticipants(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]Participant, error) {
	out := make(map[uuid.UUID][]Participant, len(ids))
	rows, err := st.db.Pool.Query(ctx, `
		SELECT p.call_id, p.user_id, p.role, p.state,
		       u.display_name, u.username, u.avatar_url
		FROM call_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.call_id = ANY($1)
		ORDER BY p.call_id, p.created_at`, ids)
	if err != nil {
		return nil, fmt.Errorf("load call participants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var callID uuid.UUID
		var p Participant
		if err := rows.Scan(&callID, &p.UserID, &p.Role, &p.State,
			&p.Profile.DisplayName, &p.Profile.Username, &p.Profile.AvatarURL); err != nil {
			return nil, fmt.Errorf("scan call participant: %w", err)
		}
		p.Profile.ID = p.UserID
		out[callID] = append(out[callID], p)
	}
	return out, rows.Err()
}
