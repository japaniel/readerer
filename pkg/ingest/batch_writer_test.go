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

	// Submit 2 items (batch size 2) - should succeed
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES (?)", "A")
		return err
	})
	bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test (val) VALUES (?)", "B")
		return err
	})

	// Close and wait for pending batches to be committed. Use a timeout to avoid hanging tests.
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- bw.Close()
	}()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for batch commit/close")
	}

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
	if err := db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count); err != nil {
		t.Fatalf("failed to query row count: %v", err)
	}
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
	// We need to ensure the committer is busy and commitCh is full when ctx is canceled.
	// Use a blocker so the committer will be processing the first batch while a second batch fills the buffer.
	bw := NewBatchWriter(nil, 1, 0) // small batch size to create batches quickly
	defer bw.Close()
	errCh := make(chan error, 1)
	bw.OnError = func(e error) {
		errCh <- e
	}

	blocker := make(chan struct{})

	// First batch: long-running callback that will block until we unblock it.
	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
		<-blocker // block here
		return nil
	}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Second batch: will be queued in commitCh buffer while first is being processed
	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Now cancel the writer's context so further batches cannot be queued
	bw.cancel()

	// Third batch: this submit will attempt to flush a batch and should find commitCh full and ctx.Done set,
	// causing it to report a dropped batch via OnError.
	if err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error { return nil }); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Unblock the first batch so committer can finish and allow Close() to complete
	close(blocker)

	// Wait for OnError to be called
	select {
	case e := <-errCh:
		if e == nil || !strings.Contains(e.Error(), "dropping batch") {
			t.Fatalf("unexpected OnError value: %v", e)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected OnError to be called when batch dropped")
	}
}
