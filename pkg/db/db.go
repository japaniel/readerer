package db

import (
	"database/sql"
	"fmt"

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
	if err := ensureColumnExists(db, "sources", "last_processed_sentence", "INTEGER DEFAULT -1"); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}

	return nil
}

func ensureColumnExists(db *sql.DB, table, column, definition string) error {
	// Check via PRAGMA table_info if the column exists
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("failed to check table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltVal interface{}
		// Scan all columns
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &pk); err != nil {
			return fmt.Errorf("failed to scan table info: %w", err)
		}
		if name == column {
			return nil // Column already exists or handled by CREATE TABLE
		}
	}

	// If scanning completes without finding the column, assume we need to add it
	if err := rows.Err(); err != nil {
		return err
	}

	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, definition)
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to add column %s: %w", column, err)
	}

	return nil
}
