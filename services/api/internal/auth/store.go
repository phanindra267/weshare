package auth

import "github.com/phanindra267/weshare/services/api/internal/database"

// Store provides authentication persistence (users, credentials, sessions).
// It wraps the shared pgx pool so auth logic stays testable in isolation.
type Store struct {
	db *database.DB
}

// NewStore builds an auth Store backed by the given pool.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}
