// Package auth implements authentication: password hashing, session
// management, rate limiting, and the HTTP handlers/middleware for the
// /api/v1/auth surface.
package auth

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

// argon2id parameters used for new hashes. Defaults come from the
// argon2id library (memory 64MB, time 1, parallelism 2, 32-byte salt,
// 32-byte key) which match OWASP's recommended minimum for Argon2id.
var argonParams = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  1,
	Parallelism: 2,
	SaltLength:  argon2id.DefaultParams.SaltLength,
	KeyLength:   argon2id.DefaultParams.KeyLength,
}

// HashPassword returns an Argon2id PHC-format hash of the password.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password must not be empty")
	}
	hash, err := argon2id.CreateHash(password, argonParams)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

// VerifyPassword checks a plaintext password against a stored PHC hash in
// constant time. It returns false (not an error) on mismatch so callers can
// present a generic failure.
func VerifyPassword(hash, password string) (bool, error) {
	if hash == "" || password == "" {
		return false, nil
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		// Malformed stored hash: treat as a mismatch rather than an
		// operational error, and never leak hash details.
		return false, nil
	}
	return ok, nil
}
