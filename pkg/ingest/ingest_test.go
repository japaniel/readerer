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
			Text: "Test",
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
