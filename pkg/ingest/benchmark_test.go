package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/readerer"
	_ "github.com/mattn/go-sqlite3"
)

func setupBenchmarkDB(b *testing.B) *sql.DB {
	// Use in-memory DB for benchmarking to isolate ingestion logic overhead somewhat
	// vs disk I/O, though SQLite in-memory still has some locking.
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatalf("failed to open db: %v", err)
	}
	// Optimize SQLite for performance to focus on application throughput
	_, _ = conn.Exec("PRAGMA synchronous = OFF")
	_, _ = conn.Exec("PRAGMA journal_mode = MEMORY")

	if err := db.InitDB(conn); err != nil {
		b.Fatalf("failed to init db: %v", err)
	}
	return conn
}

func generateBenchmarkSentences(n int) []readerer.Sentence {
	var sentences []readerer.Sentence
	for i := 0; i < n; i++ {
		// Simulate a Japanese sentence structure vaguely
		sentences = append(sentences, readerer.Sentence{
			Text: fmt.Sprintf("これはテスト文です%d", i),
			Tokens: []readerer.Token{
				{Surface: "これ", BaseForm: "これ", Reading: "コレ", PartsOfSpeech: []string{"名詞", "代名詞", "一般", "*"}},
				{Surface: "は", BaseForm: "は", Reading: "ハ", PartsOfSpeech: []string{"助詞", "係助詞", "*", "*"}},
				{Surface: "テスト", BaseForm: "テスト", Reading: "テスト", PartsOfSpeech: []string{"名詞", "サ変接続", "*", "*"}},
				{Surface: "文", BaseForm: "文", Reading: "ブン", PartsOfSpeech: []string{"名詞", "一般", "*", "*"}},
				{Surface: "です", BaseForm: "です", Reading: "デス", PartsOfSpeech: []string{"助動詞", "*", "*", "*"}},
				{Surface: fmt.Sprintf("%d", i), BaseForm: fmt.Sprintf("%d", i), Reading: fmt.Sprintf("%d", i), PartsOfSpeech: []string{"名詞", "数", "*", "*"}},
			},
		})
	}
	return sentences
}

func BenchmarkIngest(b *testing.B) {
	// 1000 sentences
	sentences := generateBenchmarkSentences(1000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		conn := setupBenchmarkDB(b)

		sourceName := fmt.Sprintf("bench_%d", i)
		sourceID, err := db.CreateOrGetSource(conn, "test", sourceName, "", "", "http://bench", "")
		if err != nil {
			conn.Close()
			b.Fatalf("CreateOrGetSource failed: %v", err)
		}

		ingester := NewIngester(conn, nil)
		ingester.Workers = 4
		ingester.BatchSize = 100
		b.StartTimer()

		_, err = ingester.Ingest(context.Background(), sourceID, sentences)
		b.StopTimer()
		if err != nil {
			conn.Close()
			b.Fatalf("Ingest failed: %v", err)
		}
		conn.Close()
	}
}

func BenchmarkIngestConcurrencyScaling(b *testing.B) {
	// Compare different worker counts.
	// Note: On small datasets or in-memory DBs, overhead of spawning workers might outweigh benefits.
	// But valid for ensuring no massive regressions.
	counts := []int{1, 2, 4, 8}
	sentences := generateBenchmarkSentences(1000)

	for _, workers := range counts {
		b.Run(fmt.Sprintf("Workers_%d", workers), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				conn := setupBenchmarkDB(b)

				sourceName := fmt.Sprintf("bench_%d_%d", workers, i)
				sourceID, err := db.CreateOrGetSource(conn, "test", sourceName, "", "", "http://bench", "")
				if err != nil {
					conn.Close()
					b.Fatalf("CreateOrGetSource failed: %v", err)
				}

				ingester := NewIngester(conn, nil)
				ingester.Workers = workers
				ingester.BatchSize = 100 // Keep batch size constant
				b.StartTimer()

				_, err = ingester.Ingest(context.Background(), sourceID, sentences)
				b.StopTimer()
				if err != nil {
					conn.Close()
					b.Fatalf("Ingest failed: %v", err)
				}
				conn.Close()
			}
		})
	}
}
