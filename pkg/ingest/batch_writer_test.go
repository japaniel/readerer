package ingest

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBatchWriterFlushesBySize(t *testing.T) {
	bw := NewBatchWriter(5, 0)
	var mu sync.Mutex
	called := 0
	for i := 0; i < 12; i++ {
		if err := bw.Submit(func(ctx context.Context) error {
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
	bw := NewBatchWriter(10, 50*time.Millisecond)
	var mu sync.Mutex
	called := 0
	if err := bw.Submit(func(ctx context.Context) error {
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
