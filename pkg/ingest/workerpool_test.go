package ingest

import (
	"context"
	"sync/atomic"
	"testing"
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
