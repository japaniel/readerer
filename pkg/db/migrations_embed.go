package db

import _ "embed"

//go:embed migrations.sql
var migrationsSQL string
