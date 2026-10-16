package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// Service holds the authentication use cases and their dependencies.
type Service struct {
	store *Store
	cfg   *config.Config

	// dummyHash is verified against when an email is unknown so that login
	// latency does not reveal whether an account exists.
	dummyHash string

	loginLimiter    *Limiter
	registerLimiter *Limiter
	passwordLimiter *Limiter
}

// SessionResult is returned by Register and Login: the raw cookie token and
// CSRF token never appear in any JSON response.
type SessionResult struct {
	User    User
	Token   string
	CSRF    string
	Session Session
}

// NewService builds an auth Service.
func NewService(store *Store, cfg *config.Config) *Service {
	dummy, _ := HashPassword("weshare-dummy-password-not-a-real-account")
	return &Service{
		store:           store,
		cfg:             cfg,
		dummyHash:       dummy,
		loginLimiter:    NewLimiter(30, 15*time.Minute),
		registerLimiter: NewLimiter(10, time.Hour),
		passwordLimiter: NewLimiter(10, time.Hour),
	}
}

// Config exposes the config for middleware/cookie helpers.
func (s *Service) Config() *config.Config { return s.cfg }

// Register creates an account and immediately starts a session.
func (s *Service) Register(ctx context.Context, in RegisterInput, meta RequestMeta) (*SessionResult, error) {
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, NewUserParams{
		Email:        in.Email,
		Username:     in.Username,
		DisplayName:  in.DisplayName,
		PasswordHash: hash,
	})
	if err != nil {
		return nil, err
	}

	return s.startSession(ctx, user, meta)
}

// Login verifies credentials and starts a session. Failures are deliberately
// indistinguishable between unknown accounts and wrong passwords.
func (s *Service) Login(ctx context.Context, in LoginInput, meta RequestMeta) (*SessionResult, error) {
	user, err := s.store.GetUserByEmail(ctx, in.Email)
	if errors.Is(err, ErrUserNotFound) {
		// Equalize timing with the "known email" path.
		_, _ = VerifyPassword(s.dummyHash, in.Password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	cred, err := s.store.GetCredential(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if cred.LockedUntil != nil && cred.LockedUntil.After(time.Now()) {
		return nil, ErrAccountLocked
	}

	ok, err := VerifyPassword(cred.PasswordHash, in.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		_, _ = s.store.RecordFailedLogin(ctx, user.ID, s.cfg.MaxLoginAttempts, s.cfg.LockoutDuration)
		return nil, ErrInvalidCredentials
	}

	if err := s.store.ResetLoginAttempts(ctx, user.ID); err != nil {
		return nil, err
	}
	return s.startSession(ctx, user, meta)
}

// Logout revokes the session identified by the raw token (best effort).
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.store.RevokeSessionByToken(ctx, token)
	return err
}

// ChangePassword verifies the current password, stores a new hash, and revokes
// every other session so stolen sessions cannot outlive a password change.
func (s *Service) ChangePassword(ctx context.Context, userID, currentSessionID uuid.UUID, current, next string) error {
	cred, err := s.store.GetCredential(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := VerifyPassword(cred.PasswordHash, current)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}

	hash, err := HashPassword(next)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.store.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.store.RevokeAllSessions(ctx, userID, &currentSessionID)
}

// ListSessions returns the user's live sessions.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID uuid.UUID) ([]Session, error) {
	return s.store.ListSessions(ctx, userID, currentSessionID)
}

// RevokeSession revokes one of the user's sessions by ID.
func (s *Service) RevokeSession(ctx context.Context, sessionID, userID uuid.UUID) error {
	return s.store.RevokeSession(ctx, sessionID, userID)
}

// RevokeOtherSessions revokes every session except the current one.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	return s.store.RevokeAllSessions(ctx, userID, &currentSessionID)
}

// GetUser loads the current user (used by /auth/me).
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (User, error) {
	return s.store.GetUserByID(ctx, id)
}

func (s *Service) startSession(ctx context.Context, user User, meta RequestMeta) (*SessionResult, error) {
	token, session, err := s.store.CreateSession(ctx, user.ID, meta.UserAgent, meta.IP, time.Now().Add(s.cfg.SessionTTL))
	if err != nil {
		return nil, err
	}
	csrf, err := newCSRFToken()
	if err != nil {
		return nil, err
	}
	return &SessionResult{User: user, Token: token, CSRF: csrf, Session: session}, nil
}

// normalizeEmail lowercases and trims an email for storage/lookup (citext
// makes this belt-and-braces, but consistent normalization avoids surprises).
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// normalizeUsername lowercases and trims a username.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
