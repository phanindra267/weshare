package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/phanindra267/weshare/services/api/internal/database"
)

// Relationship statuses stored in contacts.status.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusBlocked  = "blocked"
)

// Request is one directed contact row.
type Request struct {
	ID         uuid.UUID
	UserID     uuid.UUID // sender
	ContactID  uuid.UUID // recipient
	Status     string
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

// Contact is an accepted friendship viewed from the caller's side, joined with
// the counterpart's public profile.
type Contact struct {
	ID            uuid.UUID // of the caller's accepted row
	UserID        uuid.UUID // caller
	ContactID     uuid.UUID // friend
	DisplayName   string
	Username      *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
	AcceptedAt    time.Time
}

// RequestWithUser is a contact request joined with the counterpart's profile.
// For an incoming request the counterpart is the sender; for an outgoing one
// it is the recipient.
type RequestWithUser struct {
	Request
	DisplayName   string
	Username      *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
}

// ProfileBrief is the minimal public profile needed to render a peer.
type ProfileBrief struct {
	ID            uuid.UUID
	DisplayName   string
	Username      *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
}

const requestColumns = `id, user_id, contact_id, status, accepted_at, created_at`

func scanRequest(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.UserID, &r.ContactID, &r.Status, &r.AcceptedAt, &r.CreatedAt)
	return r, err
}

// Store provides contact persistence. Multi-statement changes run in a
// transaction so a pair can never end up half-connected.
type Store struct {
	db *database.DB
}

// NewStore builds a contacts Store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// UserExists reports whether a user id is present in the users table.
func (st *Store) UserExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := st.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("user exists: %w", err)
	}
	return exists, nil
}

// GetRequest loads a request by id.
func (st *Store) GetRequest(ctx context.Context, id uuid.UUID) (Request, error) {
	r, err := scanRequest(st.db.Pool.QueryRow(ctx,
		`SELECT `+requestColumns+` FROM contacts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrRequestNotFound
	}
	if err != nil {
		return Request{}, fmt.Errorf("get request: %w", err)
	}
	return r, nil
}

// FindRequest loads the row from user -> contact, reporting whether it exists.
func (st *Store) FindRequest(ctx context.Context, user, contact uuid.UUID) (Request, bool, error) {
	r, err := scanRequest(st.db.Pool.QueryRow(ctx,
		`SELECT `+requestColumns+` FROM contacts WHERE user_id = $1 AND contact_id = $2`,
		user, contact))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, fmt.Errorf("find request: %w", err)
	}
	return r, true, nil
}

// InsertRequest creates a pending request. A duplicate pair surfaces as
// ErrRequestAlreadySent.
func (st *Store) InsertRequest(ctx context.Context, user, contact uuid.UUID) (Request, error) {
	r, err := scanRequest(st.db.Pool.QueryRow(ctx, `
		INSERT INTO contacts (user_id, contact_id, status)
		VALUES ($1, $2, '`+StatusPending+`')
		RETURNING `+requestColumns, user, contact))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Request{}, ErrRequestAlreadySent
		}
		return Request{}, fmt.Errorf("insert request: %w", err)
	}
	return r, nil
}

// AcceptRequest marks the request accepted and materialises the reverse
// accepted row, so "are they friends" is a single-direction lookup. It fails
// with ErrRequestNotPending if the row changed concurrently and with
// ErrBlocked if the reverse side is blocked.
func (st *Store) AcceptRequest(ctx context.Context, req Request) (Request, error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return Request{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanRequest(tx.QueryRow(ctx, `
		UPDATE contacts SET status = '`+StatusAccepted+`', accepted_at = now()
		WHERE id = $1 AND status = '`+StatusPending+`'
		RETURNING `+requestColumns, req.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrRequestNotPending
	}
	if err != nil {
		return Request{}, fmt.Errorf("accept request: %w", err)
	}

	// Reverse row: insert if absent, otherwise promote to accepted. A blocked
	// reverse row is never overwritten (the WHERE clause drops the upsert).
	var reverseID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO contacts (user_id, contact_id, status, accepted_at)
		VALUES ($1, $2, '`+StatusAccepted+`', now())
		ON CONFLICT (user_id, contact_id) DO UPDATE
		SET status = '`+StatusAccepted+`', accepted_at = now()
		WHERE contacts.status <> '`+StatusBlocked+`'
		RETURNING id`, updated.ContactID, updated.UserID).Scan(&reverseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrBlocked
	}
	if err != nil {
		return Request{}, fmt.Errorf("accept reverse row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Request{}, fmt.Errorf("commit tx: %w", err)
	}
	return updated, nil
}

// DeleteRequest removes a pending request by id.
func (st *Store) DeleteRequest(ctx context.Context, id uuid.UUID) error {
	if _, err := st.db.Pool.Exec(ctx,
		`DELETE FROM contacts WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	return nil
}

// RemoveConnection drops both directions of a pair and reports how many rows
// were removed (0 means they were not connected).
func (st *Store) RemoveConnection(ctx context.Context, a, b uuid.UUID) (int64, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		DELETE FROM contacts
		WHERE (user_id = $1 AND contact_id = $2)
		   OR (user_id = $2 AND contact_id = $1)`, a, b)
	if err != nil {
		return 0, fmt.Errorf("remove connection: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Block records a block from blocker to blocked and removes the reverse row so
// the pair can never be connected while blocked.
func (st *Store) Block(ctx context.Context, blocker, blocked uuid.UUID) error {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO contacts (user_id, contact_id, status, accepted_at)
		VALUES ($1, $2, '`+StatusBlocked+`', NULL)
		ON CONFLICT (user_id, contact_id) DO UPDATE
		SET status = '`+StatusBlocked+`', accepted_at = NULL`,
		blocker, blocked); err != nil {
		return fmt.Errorf("block: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM contacts WHERE user_id = $1 AND contact_id = $2`,
		blocked, blocker); err != nil {
		return fmt.Errorf("remove reverse: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Unblock removes a block row, reporting whether one existed.
func (st *Store) Unblock(ctx context.Context, blocker, blocked uuid.UUID) (bool, error) {
	tag, err := st.db.Pool.Exec(ctx, `
		DELETE FROM contacts
		WHERE user_id = $1 AND contact_id = $2 AND status = '`+StatusBlocked+`'`,
		blocker, blocked)
	if err != nil {
		return false, fmt.Errorf("unblock: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// IsBlocked reports whether either side has blocked the other.
func (st *Store) IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var blocked bool
	err := st.db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM contacts
			WHERE status = '`+StatusBlocked+`'
			  AND ((user_id = $1 AND contact_id = $2)
			    OR (user_id = $2 AND contact_id = $1))
		)`, a, b).Scan(&blocked)
	if err != nil {
		return false, fmt.Errorf("is blocked: %w", err)
	}
	return blocked, nil
}

// GetProfileBrief loads the public profile fields for one user.
func (st *Store) GetProfileBrief(ctx context.Context, id uuid.UUID) (ProfileBrief, error) {
	var p ProfileBrief
	err := st.db.Pool.QueryRow(ctx, `
		SELECT id, display_name, username, avatar_url, status_message, last_seen_at
		FROM users WHERE id = $1`, id,
	).Scan(&p.ID, &p.DisplayName, &p.Username, &p.AvatarURL, &p.StatusMessage, &p.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProfileBrief{}, ErrUserNotFound
	}
	if err != nil {
		return ProfileBrief{}, fmt.Errorf("get profile brief: %w", err)
	}
	return p, nil
}

// cursorArg converts an optional UUID cursor into a query argument.
func cursorArg(after *uuid.UUID) any {
	if after == nil {
		return nil
	}
	return *after
}

// ListContacts returns the caller's accepted friendships (the friend's
// profile joined in), ordered by contact-row id for keyset pagination.
func (st *Store) ListContacts(ctx context.Context, me uuid.UUID, after *uuid.UUID, limit int) ([]Contact, error) {
	rows, err := st.db.Pool.Query(ctx, `
		SELECT c.id, c.user_id, c.contact_id,
		       u.display_name, u.username, u.avatar_url, u.status_message,
		       u.last_seen_at, c.accepted_at
		FROM contacts c
		JOIN users u ON u.id = c.contact_id
		WHERE c.user_id = $1 AND c.status = '`+StatusAccepted+`'
		  AND ($2::uuid IS NULL OR c.id > $2::uuid)
		ORDER BY c.id
		LIMIT $3`, me, cursorArg(after), limit)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]Contact, 0)
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.UserID, &c.ContactID, &c.DisplayName,
			&c.Username, &c.AvatarURL, &c.StatusMessage, &c.LastSeenAt,
			&c.AcceptedAt); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}
	return contacts, nil
}

// ListRequests returns pending requests for the caller. For incoming requests
// the joined profile is the sender's; for outgoing it is the recipient's.
func (st *Store) ListRequests(ctx context.Context, me uuid.UUID, incoming bool, after *uuid.UUID, limit int) ([]RequestWithUser, error) {
	// user_id is the sender in both directions, so the join target flips.
	var query string
	if incoming {
		query = `
			SELECT c.id, c.user_id, c.contact_id, c.status, c.accepted_at, c.created_at,
			       u.display_name, u.username, u.avatar_url, u.status_message, u.last_seen_at
			FROM contacts c
			JOIN users u ON u.id = c.user_id
			WHERE c.contact_id = $1 AND c.status = '` + StatusPending + `'
			  AND ($2::uuid IS NULL OR c.id > $2::uuid)
			ORDER BY c.id
			LIMIT $3`
	} else {
		query = `
			SELECT c.id, c.user_id, c.contact_id, c.status, c.accepted_at, c.created_at,
			       u.display_name, u.username, u.avatar_url, u.status_message, u.last_seen_at
			FROM contacts c
			JOIN users u ON u.id = c.contact_id
			WHERE c.user_id = $1 AND c.status = '` + StatusPending + `'
			  AND ($2::uuid IS NULL OR c.id > $2::uuid)
			ORDER BY c.id
			LIMIT $3`
	}

	rows, err := st.db.Pool.Query(ctx, query, me, cursorArg(after), limit)
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()

	requests := make([]RequestWithUser, 0)
	for rows.Next() {
		var r RequestWithUser
		if err := rows.Scan(&r.ID, &r.UserID, &r.ContactID, &r.Status,
			&r.AcceptedAt, &r.CreatedAt, &r.DisplayName, &r.Username,
			&r.AvatarURL, &r.StatusMessage, &r.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		requests = append(requests, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requests: %w", err)
	}
	return requests, nil
}
