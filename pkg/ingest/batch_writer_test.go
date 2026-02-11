package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestBatchWriterTransactions(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	bw := NewBatchWriter(db, 2, 0)
	var errs []error
	var mu sync.Mutex
	bw.OnError = func(e error) {
		mu.Lock()
		errs = append(errs, e)
		mu.Unlock()
	}

	done := make(chan struct{})
	// Submit 2 items (batch size 2) - should succeed
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES (?)", "A")
		return err
	})
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES (?)", "B")
		// Signal done after 2nd (flush should happen)
		close(done)
		return err
	})

	// Wait for async flush
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for batch execution")
	}

	// Wait a bit for the flush goroutine to finish commit (it happens after the last callback returns)
	// Ideally we'd have a signal for "flush complete" but closing logic handles it too.
	// For this test, rely on Close() to strict flush wait.
	bw.Close()

	// Verify A and B exist
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestBatchWriterRollback(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	bw := NewBatchWriter(db, 2, 0)
	errCh := make(chan error, 1)
	bw.OnError = func(e error) {
		errCh <- e
	}

	// Batch of 2: First succeeds, second fails. Whole batch should roll back.
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES (?)", "C")
		return err
	})
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		return fmt.Errorf("intentional error")
	})

	bw.Close()

	// Expect an error reported
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	default:
		t.Fatal("expected OnError to be called")
	}

	// Verify table is empty (rollback worked)
	var count int
	db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 rows (rollback), got %d", count)
	}
}

func TestBatchWriterFlushesBySize(t *testing.T) {
	bw := NewBatchWriter(nil, 5, 0)
	var mu sync.Mutex
	called := 0
	for i := 0; i < 12; i++ {
		if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
			mu.Lock()
			called++
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if called != 12 {
		t.Fatalf("expected 12 calls, got %d", called)
	}
}

func TestBatchWriterFlushesOnInterval(t *testing.T) {
	bw := NewBatchWriter(nil, 10, 50*time.Millisecond)
	var mu sync.Mutex
	called := 0
	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		mu.Lock()
		called++
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	// wait for flush interval
	time.Sleep(100 * time.Millisecond)
	if err := bw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected 1 call, got %d", called)
	}
}

func TestBatchWriterDropsBatchOnCancel(t *testing.T) {
	bw := NewBatchWriter(nil, 2, 0)
	defer bw.Close()
	errCh := make(chan error, 1)
	bw.OnError = func(e error) {
		errCh <- e
	}

	// Cancel the writer's context before submitting; this should cause flushLocked
	// to select ctx.Done() and report the dropped batch via OnError.
	bw.cancel()

	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case e := <-errCh:
		if e == nil || !strings.Contains(e.Error(), "dropping batch") {
			t.Fatalf("unexpected OnError value: %v", e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected OnError to be called when batch dropped")
	}
}
