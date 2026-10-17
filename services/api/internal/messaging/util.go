package messaging

import (
	"strings"

	"github.com/google/uuid"
)

// trimSpace is a thin alias so request-validation code reads uniformly.
func trimSpace(s string) string { return strings.TrimSpace(s) }

// parseUUID parses a canonical UUID string.
func parseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }
