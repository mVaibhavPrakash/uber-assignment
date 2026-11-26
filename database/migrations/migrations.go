// Package migrations embeds the single assignment schema migration.
package migrations

import _ "embed"

//go:embed 000001_init.up.sql
var Up string

//go:embed 000001_init.down.sql
var Down string
