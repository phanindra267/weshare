package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// User is the account record used throughout the auth layer.
type User struct {
	ID            uuid.UUID
	Username      *string
	Email         string
	DisplayName   string
	Bio           *string
	AvatarURL     *string
	StatusMessage *string
	LastSeenAt    *time.Time
	Online        bool
	Privacy       json.RawMessage
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Credential is the password/lockout record for a user.
type Credential struct {
	UserID         uuid.UUID
	PasswordHash   string
	FailedAttempts int
	LockedUntil    *time.Time
}

const userColumns = `id, username, email, display_name, bio, avatar_url,
	status_message, last_seen_at, online, privacy, created_at, updated_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Bio,
		&u.AvatarURL, &u.StatusMessage, &u.LastSeenAt, &u.Online, &u.Privacy,
		&u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// NewUserParams carries the fields needed to create an account.
type NewUserParams struct {
	Email        string
	Username     string
	DisplayName  string
	PasswordHash string
}

// CreateUser inserts a user and its credential atomically, mapping unique
// violations to ErrEmailTaken / ErrUsernameTaken.
func (st *Store) CreateUser(ctx context.Context, p NewUserParams) (User, error) {
	tx, err := st.db.Pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (email, username, display_name)
		VALUES ($1, $2, $3)
		RETURNING `+userColumns,
		p.Email, p.Username, p.DisplayName))
	if err != nil {
		return User{}, mapUniqueViolation(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_credentials (user_id, password_hash)
		VALUES ($1, $2)`, user.ID, p.PasswordHash); err != nil {
		return User{}, fmt.Errorf("insert credential: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit tx: %w", err)
	}
	return user, nil
}

// GetUserByID loads a user by primary key.
func (st *Store) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	user, err := scanUser(st.db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

// GetUserByEmail loads a user by (case-insensitive) email.
func (st *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	user, err := scanUser(st.db.Pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

// GetCredential loads the credential record for a user.
func (st *Store) GetCredential(ctx context.Context, userID uuid.UUID) (Credential, error) {
	var c Credential
	err := st.db.Pool.QueryRow(ctx, `
		SELECT user_id, password_hash, failed_attempts, locked_until
		FROM user_credentials WHERE user_id = $1`, userID,
	).Scan(&c.UserID, &c.PasswordHash, &c.FailedAttempts, &c.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrUserNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("get credential: %w", err)
	}
	return c, nil
}

// RecordFailedLogin increments the failure counter and locks the account once
// maxAttempts is reached. It returns whether the account is now locked.
func (st *Store) RecordFailedLogin(ctx context.Context, userID uuid.UUID, maxAttempts int, lockFor time.Duration) (bool, error) {
	var lockedUntil *time.Time
	err := st.db.Pool.QueryRow(ctx, `
		UPDATE user_credentials
		SET failed_attempts = failed_attempts + 1,
		    locked_until = CASE
		        WHEN failed_attempts + 1 >= $2 THEN now() + $3::interval
		        ELSE locked_until
		    END
		WHERE user_id = $1
		RETURNING locked_until`,
		userID, maxAttempts, lockFor.String(),
	).Scan(&lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("record failed login: %w", err)
	}
	return lockedUntil != nil && lockedUntil.After(time.Now()), nil
}

// ResetLoginAttempts clears failure/lockout state after a successful login.
func (st *Store) ResetLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	_, err := st.db.Pool.Exec(ctx, `
		UPDATE user_credentials
		SET failed_attempts = 0, locked_until = NULL
		WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("reset login attempts: %w", err)
	}
	return nil
}

// UpdatePassword replaces the password hash and clears lockout state.
func (st *Store) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	tag, err := st.db.Pool.Exec(ctx, `
		UPDATE user_credentials
		SET password_hash = $2,
		    password_updated_at = now(),
		    failed_attempts = 0,
		    locked_until = NULL
		WHERE user_id = $1`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// mapUniqueViolation converts a Postgres 23505 error into a domain error.
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch {
		case strings.Contains(pgErr.ConstraintName, "username"):
			return ErrUsernameTaken
		case strings.Contains(pgErr.ConstraintName, "email"):
			return ErrEmailTaken
		}
	}
	return fmt.Errorf("create user: %w", err)
}
