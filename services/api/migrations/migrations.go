// Package migrations embeds the SQL migration files so the server binary is
// self-contained and can apply schema changes without external tooling.
package migrations

import "embed"

// FS contains every `*.up.sql` and `*.down.sql` migration, keyed by filename.
//
//go:embed *.sql
var FS embed.FS
