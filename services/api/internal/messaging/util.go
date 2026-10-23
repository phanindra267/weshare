package messaging

import (
	"strings"

	"github.com/google/uuid"
)

// trimSpace is a thin alias so request-validation code reads uniformly.
func trimSpace(s string) string { return strings.TrimSpace(s) }

// parseUUID parses a canonical UUID string.
func parseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// escapeLike makes a user-supplied substring safe to embed in an ILIKE
// pattern: the LIKE metacharacters are escaped so they match literally. The
// queries pair this with SQL `ESCAPE '\'`.
func escapeLike(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
