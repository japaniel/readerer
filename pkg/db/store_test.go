package db

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Ensure single connection to avoid separate in-memory DBs per connection.
	// NOTE: This serializes all DB operations through a single connection, which means
	// concurrency tests below don't test true parallel execution, but rather the
	// correctness of the logic under simulated concurrent access patterns.
	db.SetMaxOpenConns(1)
	if err := InitDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateOrGetWord(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	id1, err := CreateOrGetWord(db, "犬", "犬", "いぬ", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	id2, err := CreateOrGetWord(db, "犬", "犬", "いぬ", "", "ja")
	if err != nil {
		t.Fatalf("get word: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id, got %d and %d", id1, id2)
	}
}

func TestCreateOrGetSource(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	id1, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/a", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	id2, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/a", "")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same source id, got %d and %d", id1, id2)
	}
}

func TestLinkAndQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	wID, err := CreateOrGetWord(db, "猫", "猫", "ねこ", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	sID, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/b", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "この猫は可愛い。", "この猫は可愛い。", 1); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Link again to test occurrence_count increment via upsert
	if err := LinkWordToSource(db, wID, sID, "この猫は可愛い。", "この猫は可愛い。", 1); err != nil {
		t.Fatalf("link 2: %v", err)
	}
	// verify occurrence_count
	var cnt int
	err = db.QueryRow(`SELECT occurrence_count FROM word_sources WHERE word_id = ? AND source_id = ?`, wID, sID).Scan(&cnt)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("expected occurrence_count=2, got %d", cnt)
	}

	words, err := GetWordsBySource(db, sID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(words) != 1 {
		t.Fatalf("expected 1 word, got %d", len(words))
	}
	if words[0].Word != "猫" {
		t.Fatalf("expected 猫, got %s", words[0].Word)
	}
}

func TestCreateOrGetWordConcurrency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	const n = 8
	ids := make(chan int64, n)
	for i := 0; i < n; i++ {
		go func() {
			id, err := CreateOrGetWord(db, "犬", "犬", "いぬ", "", "ja")
			if err != nil {
				t.Errorf("create or get word: %v", err)
				ids <- 0
				return
			}
			ids <- id
		}()
	}
	var first int64
	for i := 0; i < n; i++ {
		id := <-ids
		if id == 0 {
			t.Fatalf("error in goroutine")
		}
		if i == 0 {
			first = id
		}
		if id != first {
			t.Fatalf("expected same id, got %d and %d", first, id)
		}
	}
	// ensure only one row exists
	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE word = ? AND lemma = ?`, "犬", "犬").Scan(&cnt)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected 1 word row, got %d", cnt)
	}
}

func TestCreateOrGetSourceConcurrency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	const n = 8
	ids := make(chan int64, n)
	for i := 0; i < n; i++ {
		go func() {
			id, err := CreateOrGetSource(db, "website_article", "Title", "Author", "example.com", "https://example.com/c", "")
			if err != nil {
				t.Errorf("create or get source: %v", err)
				ids <- 0
				return
			}
			ids <- id
		}()
	}
	var first int64
	for i := 0; i < n; i++ {
		id := <-ids
		if id == 0 {
			t.Fatalf("error in goroutine")
		}
		if i == 0 {
			first = id
		}
		if id != first {
			t.Fatalf("expected same id, got %d and %d", first, id)
		}
	}
	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM sources WHERE url = ?`, "https://example.com/c").Scan(&cnt)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected 1 source row, got %d", cnt)
	}
}

func TestCreateOrGetWordEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, err := CreateOrGetWord(db, "  ", "", "", "", "ja")
	if err == nil {
		t.Fatalf("expected error for empty word")
	}
}

func TestGetWordsBySourceNullCols(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	wID, err := CreateOrGetWord(db, "魚", "魚", "さかな", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	sID, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/d", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "その魚は速い。", "その魚は速い。", 1); err != nil {
		t.Fatalf("link: %v", err)
	}
	words, err := GetWordsBySource(db, sID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(words) != 1 {
		t.Fatalf("expected 1 word, got %d", len(words))
	}
	if words[0].Pronunciation != "さかな" {
		t.Fatalf("expected pronunciation 'さかな', got %s", words[0].Pronunciation)
	}
	if words[0].ImageURL != "" {
		t.Fatalf("expected empty image url, got %s", words[0].ImageURL)
	}
}

func TestLinkUpdatesContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	wID, err := CreateOrGetWord(db, "鳥", "鳥", "とり", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	sID, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/e", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "最初の文。", "最初の文。", 1); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "更新された文。", "更新された文。", 1); err != nil {
		t.Fatalf("link update: %v", err)
	}
	var ctxID, exID sql.NullInt64
	err = db.QueryRow(`SELECT context_sentence_id, example_sentence_id FROM word_sources WHERE word_id = ? AND source_id = ?`, wID, sID).Scan(&ctxID, &exID)
	if err != nil {
		t.Fatalf("query ids: %v", err)
	}
	if !ctxID.Valid || !exID.Valid {
		t.Fatalf("expected non-null sentence ids, got %v / %v", ctxID, exID)
	}
	var ctx, ex string
	err = db.QueryRow(`SELECT text FROM sentences WHERE id = ?`, ctxID.Int64).Scan(&ctx)
	if err != nil {
		t.Fatalf("query ctx text: %v", err)
	}
	err = db.QueryRow(`SELECT text FROM sentences WHERE id = ?`, exID.Int64).Scan(&ex)
	if err != nil {
		t.Fatalf("query ex text: %v", err)
	}
	if ctx != "更新された文。" || ex != "更新された文。" {
		t.Fatalf("expected updated context/example, got %s / %s", ctx, ex)
	}
}

func TestCreateOrGetSourceEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, err := CreateOrGetSource(db, "  ", "", "", "", "", "")
	if err == nil {
		t.Fatalf("expected error for empty sourceType")
	}
}

func TestLinkWordToSourceInvalidIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Test with invalid wordID
	err := LinkWordToSource(db, 0, 1, "context", "example", 1)
	if err == nil {
		t.Fatalf("expected error for wordID <= 0")
	}

	// Test with invalid sourceID
	err = LinkWordToSource(db, 1, 0, "context", "example", 1)
	if err == nil {
		t.Fatalf("expected error for sourceID <= 0")
	}

	// Test with negative wordID
	err = LinkWordToSource(db, -1, 1, "context", "example", 1)
	if err == nil {
		t.Fatalf("expected error for negative wordID")
	}
}

func TestDefinitionsPersistence(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	wID, err := CreateOrGetWord(db, "試験", "試験", "しけん", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	// set definitions JSON
	defsJSON := `[{"sense":"test sense"}]`
	if err := UpdateWordDefinitions(db, wID, defsJSON); err != nil {
		t.Fatalf("update definitions: %v", err)
	}
	// link to a source and query
	sID, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/defs", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "文脈", "例文", 1); err != nil {
		t.Fatalf("link: %v", err)
	}
	words, err := GetWordsBySource(db, sID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(words) != 1 {
		t.Fatalf("expected 1 word, got %d", len(words))
	}
	if words[0].Definitions != defsJSON {
		t.Fatalf("expected definitions %s, got %s", defsJSON, words[0].Definitions)
	}
}

func TestLinkWordToSource_ContextLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	wID, err := CreateOrGetWord(db, "TestWord", "Lemma", "Reading", "", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}

	sID, err := CreateOrGetSource(db, "website", "Title", "Author", "Site", "http://example.com/limit", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	// Insert 7 different contexts
	for i := 1; i <= 7; i++ {
		ctx := fmt.Sprintf("Context sentence %d.", i)
		err := LinkWordToSource(db, wID, sID, ctx, ctx, 1)
		if err != nil {
			t.Fatalf("link iteration %d: %v", i, err)
		}
	}

	// Verify occurrence count is 7
	var count int
	err = db.QueryRow("SELECT occurrence_count FROM word_sources WHERE word_id = ? AND source_id = ?", wID, sID).Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 7 {
		t.Errorf("expected occurrence_count=7, got %d", count)
	}

	// Verify context lines count is 5
	// We need to resolve word_source_id first
	var wsID int64
	err = db.QueryRow("SELECT id FROM word_sources WHERE word_id = ? AND source_id = ?", wID, sID).Scan(&wsID)
	if err != nil {
		t.Fatalf("get word_source_id: %v", err)
	}

	var ctxCount int
	err = db.QueryRow("SELECT COUNT(*) FROM word_contexts WHERE word_source_id = ?", wsID).Scan(&ctxCount)
	if err != nil {
		t.Fatalf("query distinct contexts: %v", err)
	}
	if ctxCount != 5 {
		t.Errorf("expected 5 stored contexts, got %d", ctxCount)
	}
}
