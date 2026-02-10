package ingest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/readerer"
	_ "github.com/mattn/go-sqlite3"
)

func setupDB(t *testing.T) *sql.DB {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := db.InitDB(conn); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return conn
}

func TestIngestResume(t *testing.T) {
	conn := setupDB(t)
	defer conn.Close()

	// Create a source
	sourceID, err := db.CreateOrGetSource(conn, "test", "Title", "Author", "Site", "http://test", "")
	if err != nil {
		t.Fatal(err)
	}

	// Prepare 10 dummy sentences
	var sentences []readerer.Sentence
	for i := 0; i < 10; i++ {
		sentences = append(sentences, readerer.Sentence{
			Text: "テスト",
			Tokens: []readerer.Token{
				{Surface: "テスト", BaseForm: "テスト", Reading: "テスト", PartsOfSpeech: []string{"名詞"}},
			},
		})
	}

	// Manually set progress to index 4 (so 5 sentences processed: 0,1,2,3,4)
	if err := db.UpdateSourceProgress(conn, sourceID, 4); err != nil {
		t.Fatal(err)
	}

	ingester := NewIngester(conn, nil)
	ingester.BatchSize = 2 // Verify batching doesn't interfere

	// Ingest
	count, err := ingester.Ingest(context.Background(), sourceID, sentences)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	// We expect sentences 5,6,7,8,9 to be processed. (5 items).
	if count != 5 {
		t.Errorf("Expected 5 linked items, got %d", count)
	}
}

func TestIngestContextCancel(t *testing.T) {
	conn := setupDB(t)
	defer conn.Close()
	sourceID, _ := db.CreateOrGetSource(conn, "test", "Title", "", "", "http://test2", "")

	sentences := make([]readerer.Sentence, 100)
	for i := range sentences {
		sentences[i] = readerer.Sentence{
			Text:   "Test",
			Tokens: []readerer.Token{{Surface: "A", BaseForm: "A", Reading: "A", PartsOfSpeech: []string{"Noun"}}},
		}
	}

	ingester := NewIngester(conn, nil)
	ingester.BatchSize = 10

	// Create a context that is ALREADY canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count, err := ingester.Ingest(ctx, sourceID, sentences)

	// Should return ctx.Err() immediately or very quickly.
	// Logic: Ingest check select { case <-ctx.Done(): ... } at start of loop.
	// It should process 0 items.

	if count != 0 {
		t.Errorf("Expected 0 linked items with cancelled context, got %d", count)
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestIngestNormalizationAndFiltering(t *testing.T) {
	conn := setupDB(t)
	defer conn.Close()

	sourceID, err := db.CreateOrGetSource(conn, "test", "NormTitle", "Author", "Site", "http://norm", "")
	if err != nil {
		t.Fatal(err)
	}

	tokens := []readerer.Token{
		{Surface: "手紙", BaseForm: "手紙", Reading: "テガミ", PartsOfSpeech: []string{"名詞"}, PrimaryPOS: "名詞"},
		{Surface: "を", BaseForm: "を", Reading: "ヲ", PartsOfSpeech: []string{"助詞"}, PrimaryPOS: "助詞"},
		{Surface: "書い", BaseForm: "書く", Reading: "カイ", PartsOfSpeech: []string{"動詞"}, PrimaryPOS: "動詞"},
		{Surface: "まし", BaseForm: "ます", Reading: "マシ", PartsOfSpeech: []string{"助動詞"}, PrimaryPOS: "助動詞"},
	}

	sentences := []readerer.Sentence{
		{Text: "手紙を書いました", Tokens: tokens},
	}

	ingester := NewIngester(conn, nil)
	count, err := ingester.Ingest(context.Background(), sourceID, sentences)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	// We expect 2 words linked: "手紙" and "書く".
	// "を" and "まし" should be filtered out.

	if count != 2 {
		t.Errorf("Expected 2 linked words, got %d", count)
	}

	// Verify DB contents
	rows, err := conn.Query("SELECT word FROM words ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			t.Fatal(err)
		}
		words = append(words, w)
	}

	expected := []string{"手紙", "書く"}
	if len(words) != len(expected) {
		t.Fatalf("Expected %d words in DB, got %d: %v", len(expected), len(words), words)
	}
	for i, w := range words {
		if w != expected[i] {
			t.Errorf("Expected word %d to be %s, got %s", i, expected[i], w)
		}
	}
}

func TestIngestDuplicateContext(t *testing.T) {
conn := setupDB(t)
defer conn.Close()

sourceID, err := db.CreateOrGetSource(conn, "test", "DuplicateTest", "Author", "Site", "http://dup", "")
if err != nil {
t.Fatal(err)
}

sentenceText := "猫は猫である"
sentences := []readerer.Sentence{
{
Text: sentenceText,
Tokens: []readerer.Token{
{Surface: "猫", BaseForm: "猫", Reading: "ネコ", PartsOfSpeech: []string{"名詞"}, PrimaryPOS: "名詞"},
{Surface: "は", BaseForm: "は", Reading: "ハ", PartsOfSpeech: []string{"助詞"}, PrimaryPOS: "助詞"},
{Surface: "猫", BaseForm: "猫", Reading: "ネコ", PartsOfSpeech: []string{"名詞"}, PrimaryPOS: "名詞"},
},
},
}

ingester := NewIngester(conn, nil)
ingester.BatchSize = 10

countProcessed, err := ingester.Ingest(context.Background(), sourceID, sentences)
if err != nil {
t.Fatalf("Ingest failed: %v", err)
}

if countProcessed != 2 {
t.Errorf("Expected 2 processed tokens, got %d", countProcessed)
}

// Helper to get counts
var wordSourceID int64
var count int
err = conn.QueryRow(`
SELECT ws.id, ws.occurrence_count 
FROM word_sources ws 
JOIN words w ON ws.word_id = w.id 
WHERE w.word = '猫' AND ws.source_id = ?`, sourceID).Scan(&wordSourceID, &count)

if err != nil {
t.Fatalf("Failed to query word_sources: %v", err)
}

if count != 2 {
t.Errorf("Expected occurrence_count 2 for '猫', got %d", count)
}

// Check contexts
var contextCount int
err = conn.QueryRow(`SELECT COUNT(*) FROM word_contexts WHERE word_source_id = ?`, wordSourceID).Scan(&contextCount)
if err != nil {
t.Fatalf("Failed to query word_contexts: %v", err)
}

if contextCount != 1 {
t.Errorf("Expected 1 context sentence, got %d", contextCount)
}
}
