package migrations

import "embed"

// FS embeds all SQL migration files for distribution and testing.
//
//go:embed *.sql
var FS embed.FS
