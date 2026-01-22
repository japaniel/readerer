package db

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := InitDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateOrGetWord(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	id1, err := CreateOrGetWord(db, "犬", "犬", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	id2, err := CreateOrGetWord(db, "犬", "犬", "ja")
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
	wID, err := CreateOrGetWord(db, "猫", "猫", "ja")
	if err != nil {
		t.Fatalf("create word: %v", err)
	}
	sID, err := CreateOrGetSource(db, "website_article", "", "", "example.com", "https://example.com/b", "")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := LinkWordToSource(db, wID, sID, "この猫は可愛い。", "この猫は可愛い。"); err != nil {
		t.Fatalf("link: %v", err)
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
