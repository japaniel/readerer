package ingest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolRunsJobs(t *testing.T) {
	p := NewWorkerPool(4, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	var ran int32
	jobs := 100
	for i := 0; i < jobs; i++ {
		err := p.Submit(func(ctx context.Context) error {
			atomic.AddInt32(&ran, 1)
			return nil
		})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}
	// close and wait
	p.Close()

	if got := atomic.LoadInt32(&ran); int(got) != jobs {
		t.Fatalf("expected %d jobs executed, got %d", jobs, got)
	}
}

func TestSubmitAfterClose(t *testing.T) {
	p := NewWorkerPool(1, 2)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	p.Close()
	cancel()
	if err := p.Submit(func(ctx context.Context) error { return nil }); err == nil {
		t.Fatalf("expected error submitting to closed pool")
	}
}

func TestSubmitRecoversFromCloseRace(t *testing.T) {
	p := NewWorkerPool(1, 1) // capacity 1
	// don't start workers so the second Submit blocks when queue is full
	// fill the queue
	if err := p.Submit(func(ctx context.Context) error { return nil }); err != nil {
		t.Fatalf("setup submit failed: %v", err)
	}

	// start a goroutine that will block trying to submit the second job
	done := make(chan error, 1)
	go func() {
		err := p.Submit(func(ctx context.Context) error { return nil })
		done <- err
	}()

	// give the goroutine time to block on the full queue
	time.Sleep(10 * time.Millisecond)

	// close the pool which should cause the blocked Submit to return ErrPoolClosed
	p.Close()

	err := <-done
	if err == nil {
		t.Fatalf("expected error submitting to closed pool, got nil")
	}
	if err != ErrPoolClosed {
		t.Fatalf("expected ErrPoolClosed, got %v", err)
	}
}

func TestContextCancellationStopsWorkers(t *testing.T) {
	p := NewWorkerPool(2, 16)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	// Cancel the context while workers are idle and ensure Close() returns promptly
	cancel()
	done := make(chan struct{}, 1)
	go func() {
		p.Close()
		done <- struct{}{}
	}()

	select {
	case <-done:
		// success — workers exited after context cancellation
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Close blocked after context cancellation")
	}
}
