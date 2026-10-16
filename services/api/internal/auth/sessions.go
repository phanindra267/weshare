package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Session is a persisted browser session.
type Session struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	UserAgent  string     `json:"user_agent,omitempty"`
	IP         string     `json:"ip,omitempty"`
	LastUsedAt time.Time  `json:"last_used_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	IsCurrent  bool       `json:"is_current"`
}

var (
	// ErrSessionNotFound indicates no matching live session.
	ErrSessionNotFound = errors.New("session not found")
	// ErrInvalidToken indicates a malformed session token.
	ErrInvalidToken = errors.New("invalid session token")
)

const tokenBytes = 32

// newSessionToken returns a random 256-bit session token (base64url) and its
// hex SHA-256 hash for storage.
func newSessionToken() (token, tokenHash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	tokenHash = hashToken(token)
	return token, tokenHash, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

// CreateSession persists a new session and returns the raw token (which must
// only be placed in an HTTP-only cookie, never in a response body).
func (st *Store) CreateSession(
	ctx context.Context,
	userID uuid.UUID,
	userAgent, ip string,
	expiresAt time.Time,
) (token string, session Session, err error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", Session{}, err
	}

	var ipNet *net.IPNet
	if parsed := net.ParseIP(ip); parsed != nil {
		ipNet = &net.IPNet{IP: parsed, Mask: parsed.DefaultMask()}
	}

	err = st.db.Pool.QueryRow(ctx, `
		INSERT INTO sessions (token_hash, user_id, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, COALESCE(user_agent,''), COALESCE(host(ip),''),
		          last_used_at, expires_at, revoked_at, created_at`,
		tokenHash, userID, userAgent, ipNet, expiresAt,
	).Scan(&session.ID, &session.UserID, &session.UserAgent, &session.IP,
		&session.LastUsedAt, &session.ExpiresAt, &session.RevokedAt,
		&session.CreatedAt)
	if err != nil {
		return "", Session{}, fmt.Errorf("insert session: %w", err)
	}
	session.IsCurrent = true
	return token, session, nil
}

// LookupSession resolves a raw token to a live session and its user, updates
// last_used_at, and returns ErrSessionNotFound for expired/revoked sessions.
func (st *Store) LookupSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidToken
	}
	tokenHash := hashToken(token)

	var s Session
	err := st.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, COALESCE(user_agent,''), COALESCE(host(ip),''),
		       last_used_at, expires_at, revoked_at, created_at
		FROM sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()`,
		tokenHash,
	).Scan(&s.ID, &s.UserID, &s.UserAgent, &s.IP, &s.LastUsedAt,
		&s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}

	// Best-effort freshness bookkeeping; failures are not fatal.
	_, _ = st.db.Pool.Exec(ctx,
		`UPDATE sessions SET last_used_at = now() WHERE id = $1`, s.ID)
	s.LastUsedAt = time.Now()

	return s, nil
}

// RevokeSession revokes a session. It is a no-op when the session does not
// belong to userID (prevents cross-user revocation).
func (st *Store) RevokeSession(ctx context.Context, sessionID, userID uuid.UUID) error {
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeSessionByToken revokes the session identified by a raw token and
// returns whether anything was revoked.
func (st *Store) RevokeSessionByToken(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`,
		hashToken(token))
	if err != nil {
		return false, fmt.Errorf("revoke session by token: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeAllSessions revokes every live session for a user, optionally keeping
// the session identified by keepSessionID (used by "change password").
func (st *Store) RevokeAllSessions(ctx context.Context, userID uuid.UUID, keepSessionID *uuid.UUID) error {
	query := `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	args := []any{userID}
	if keepSessionID != nil {
		query += ` AND id <> $2`
		args = append(args, *keepSessionID)
	}
	if _, err := st.db.Pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}

// ListSessions returns all non-expired sessions for a user, newest first.
// currentSessionID is flagged so the UI can show "this device".
func (st *Store) ListSessions(ctx context.Context, userID, currentSessionID uuid.UUID) ([]Session, error) {
	rows, err := st.db.Pool.Query(ctx, `
		SELECT id, user_id, COALESCE(user_agent,''), COALESCE(host(ip),''),
		       last_used_at, expires_at, revoked_at, created_at
		FROM sessions
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	out := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.UserAgent, &s.IP,
			&s.LastUsedAt, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.IsCurrent = s.ID == currentSessionID
		out = append(out, s)
	}
	return out, rows.Err()
}
