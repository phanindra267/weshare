// Package users exposes profile read/update endpoints and avatar upload.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// ErrNotFound indicates a missing user.
var ErrNotFound = errors.New("user not found")

// Profile is the client-facing account profile (email is intentionally
// excluded here; the owner's email is served by /auth/me).
type Profile struct {
	ID            uuid.UUID
	Username      *string
	DisplayName   string
	Bio           *string
	AvatarURL     *string
	StatusMessage *string
	Online        bool
	LastSeenAt    *time.Time
	CreatedAt     time.Time
}

const profileColumns = `id, username, display_name, bio, avatar_url,
	status_message, online, last_seen_at, created_at`

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Username, &p.DisplayName, &p.Bio, &p.AvatarURL,
		&p.StatusMessage, &p.Online, &p.LastSeenAt, &p.CreatedAt)
	return p, err
}

// Store provides profile persistence.
type Store struct {
	db *database.DB
}

// NewStore builds a users Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// GetByID loads a public profile.
func (st *Store) GetByID(ctx context.Context, id uuid.UUID) (Profile, error) {
	p, err := scanProfile(st.db.Pool.QueryRow(ctx,
		`SELECT `+profileColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return p, nil
}

// Update carries optional fields; nil means "leave unchanged". For Bio and
// StatusMessage, a pointer to an empty string clears the field (stored NULL).
type Update struct {
	DisplayName   *string
	Bio           *string
	StatusMessage *string
}

// UpdateProfile applies a partial update and returns the fresh profile.
func (st *Store) UpdateProfile(ctx context.Context, id uuid.UUID, upd Update) (Profile, error) {
	sets := make([]string, 0, 3)
	args := []any{id}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if upd.DisplayName != nil {
		sets = append(sets, "display_name = "+arg(*upd.DisplayName))
	}
	if upd.Bio != nil {
		sets = append(sets, "bio = NULLIF("+arg(*upd.Bio)+", '')")
	}
	if upd.StatusMessage != nil {
		sets = append(sets, "status_message = NULLIF("+arg(*upd.StatusMessage)+", '')")
	}

	if len(sets) == 0 {
		return st.GetByID(ctx, id)
	}

	query := `UPDATE users SET ` + strings.Join(sets, ", ") +
		` WHERE id = $1 RETURNING ` + profileColumns

	p, err := scanProfile(st.db.Pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	return p, nil
}

// SetAvatarURL points the user's avatar at a stored object.
func (st *Store) SetAvatarURL(ctx context.Context, id uuid.UUID, url string) (Profile, error) {
	p, err := scanProfile(st.db.Pool.QueryRow(ctx, `
		UPDATE users SET avatar_url = $2 WHERE id = $1
		RETURNING `+profileColumns, id, url))
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("set avatar: %w", err)
	}
	return p, nil
}

// TouchPresence records a heartbeat and returns the new last-seen timestamp.
// Whether the user reads as online is derived from this timestamp against the
// configured staleness window, not from the stored boolean.
func (st *Store) TouchPresence(ctx context.Context, id uuid.UUID) (time.Time, error) {
	var lastSeen time.Time
	err := st.db.Pool.QueryRow(ctx, `
		UPDATE users SET last_seen_at = now(), online = TRUE
		WHERE id = $1
		RETURNING last_seen_at`, id).Scan(&lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("touch presence: %w", err)
	}
	return lastSeen, nil
}
