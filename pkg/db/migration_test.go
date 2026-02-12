package db

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestInitDBCreatesFinalSchema verifies InitDB creates the final, id-based
// schema (no legacy text columns are created) so fresh DBs have the expected
// tables and columns.
func TestInitDBCreatesFinalSchema(t *testing.T) {
	dbConn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbConn.Close()

	if err := InitDB(dbConn); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Verify `sentences` table exists
	var name string
	if err := dbConn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sentences'").Scan(&name); err != nil {
		t.Fatalf("sentences table missing: %v", err)
	}

	// Verify word_sources has id-based sentence columns
	rows, err := dbConn.Query("PRAGMA table_info(word_sources)")
	if err != nil {
		t.Fatalf("pragmas: %v", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var colName, ctype string
		var notnull, pk int
		var dfltVal interface{}
		if err := rows.Scan(&cid, &colName, &ctype, &notnull, &dfltVal, &pk); err != nil {
			t.Fatalf("scan col: %v", err)
		}
		cols[colName] = true
	}
	if !cols["context_sentence_id"] || !cols["example_sentence_id"] {
		t.Fatalf("expected context_sentence_id and example_sentence_id in word_sources, got %v", cols)
	}

	// Verify word_contexts uses sentence_id
	rows2, err := dbConn.Query("PRAGMA table_info(word_contexts)")
	if err != nil {
		t.Fatalf("pragmas: %v", err)
	}
	defer rows2.Close()
	cols2 := map[string]bool{}
	for rows2.Next() {
		var cid int
		var colName, ctype string
		var notnull, pk int
		var dfltVal interface{}
		if err := rows2.Scan(&cid, &colName, &ctype, &notnull, &dfltVal, &pk); err != nil {
			t.Fatalf("scan col: %v", err)
		}
		cols2[colName] = true
	}
	if !cols2["sentence_id"] {
		t.Fatalf("expected sentence_id in word_contexts, got %v", cols2)
	}
}
