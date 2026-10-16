package auth

import "errors"

// Domain errors returned by the auth service. Handlers translate these into
// HTTP responses via httpx; persistence-level errors are wrapped and logged.
var (
	// ErrInvalidCredentials is returned for any failed login. It is
	// intentionally identical for "unknown account" and "wrong password" so
	// the API never discloses whether an email is registered.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrAccountLocked is returned when failed attempts exceed the limit.
	ErrAccountLocked = errors.New("account temporarily locked")
	// ErrEmailTaken is returned when registering an existing email.
	ErrEmailTaken = errors.New("email already registered")
	// ErrUsernameTaken is returned when registering an existing username.
	ErrUsernameTaken = errors.New("username already taken")
	// ErrUserNotFound is returned when a user lookup misses.
	ErrUserNotFound = errors.New("user not found")
)
