package migrations

import "embed"

// Files contains the initial database schema.
//
//go:embed *.sql
var Files embed.FS
