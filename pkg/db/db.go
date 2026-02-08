package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB runs migrations on the given DB connection using the embedded SQL.
// We execute the full SQL batch so that statement parsing is delegated to SQLite
// (safer than naive semicolon-splitting which can break on semicolons inside
// strings or comments).
//
// For more complex migration needs (versioning, rollbacks), consider using a
// migration library (e.g., golang-migrate) in the future. For the current
// scope the embedded SQL is sufficient and simplified for tests.
func InitDB(db *sql.DB) error {
	// Ensure SQLite enforces foreign key constraints on this connection.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	_, err := db.Exec(migrationsSQL)
	if err != nil {
		return err
	}
	
	// Migration for existing databases (add last_processed_sentence if missing)
	// We ignore the error if column exists
	_, _ = db.Exec("ALTER TABLE sources ADD COLUMN last_processed_sentence INTEGER DEFAULT -1;")
	
	return nil
}
