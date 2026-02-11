package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/readerer"
	_ "github.com/mattn/go-sqlite3"
)

// failingPool always returns an error on Submit to simulate producer error.
type failingPool struct{}

func (f *failingPool) Start(ctx context.Context) {}
func (f *failingPool) Submit(job Job) error      { return errors.New("submit failed") }
func (f *failingPool) SubmitCtx(ctx context.Context, job Job) error {
	return errors.New("submit failed")
}
func (f *failingPool) Close() {}
func TestIngestHandlesSubmitErrorClosesResultCh(t *testing.T) {
	conn := setupDB(t)
	defer conn.Close()

	sourceID, err := db.CreateOrGetSource(conn, "test", "SubmitError", "", "", "http://submit", "")
	if err != nil {
		t.Fatal(err)
	}

	// Prepare a few sentences
	sentences := make([]readerer.Sentence, 10)
	for i := range sentences {
		sentences[i] = readerer.Sentence{
			Text:   "テスト",
			Tokens: []readerer.Token{{Surface: "A", BaseForm: "A", Reading: "A", PartsOfSpeech: []string{"名詞"}}},
		}
	}

	ingester := NewIngester(conn, nil)
	// Inject failing pool so first Submit() returns an error
	ingester.PoolFactory = func(workers, queue int) WorkerPoolInterface { return &failingPool{} }

	// Run ingest and expect it to return quickly with the submit error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = ingester.Ingest(ctx, sourceID, sentences)
	if err == nil {
		t.Fatalf("expected submit error, got nil")
	}
}
