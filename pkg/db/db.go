package db

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB runs migrations on the given DB connection using the embedded SQL.
func InitDB(db *sql.DB) error {
	stmts := strings.Split(migrationsSQL, ";")
	for _, s := range stmts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
