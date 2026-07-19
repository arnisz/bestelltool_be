package migrations

import "embed"

// Files enthält alle SQL-Migrationen, die ins Binary eingebettet werden.
//
//go:embed *.sql
var Files embed.FS
