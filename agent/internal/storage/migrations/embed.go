// Package migrations bundles the goose SQL migrations for the agent's
// MySQL database as an embedded filesystem.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
