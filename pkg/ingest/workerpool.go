package ingest

import (
	"context"
	"sync"
)

// Job is a unit of work submitted to the WorkerPool.
// It returns an error to indicate failure; callers may treat errors as they see fit.
type Job func(ctx context.Context) error

// WorkerPool runs jobs using a fixed number of goroutines.
// It is intentionally lightweight and designed to be integrated into
// the Ingester to parallelize CPU-bound work (tokenization, dictionary lookup).
type WorkerPool struct {
	jobs    chan Job
	wg      sync.WaitGroup
	workers int
	closeMu sync.Mutex
	closed  bool
}

// NewWorkerPool creates a new worker pool with the specified number of workers
// and job queue capacity.
func NewWorkerPool(workers, queue int) *WorkerPool {
	if workers <= 0 {
		workers = 1
	}
	if queue <= 0 {
		queue = workers * 2
	}
	p := &WorkerPool{
		jobs:    make(chan Job, queue),
		workers: workers,
	}
	return p
}

// Start begins the worker goroutines and listens for jobs until ctx is done or Close is called.
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					// Run job and ignore error — caller can handle via shared channels / DB state
					_ = job(ctx)
				}
			}
		}()
	}
}

// Submit enqueues a job for processing. Returns an error if the pool is closed.
func (p *WorkerPool) Submit(job Job) error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return ErrPoolClosed
	}
	p.jobs <- job
	return nil
}

// Close stops accepting new jobs and waits for workers to finish.
func (p *WorkerPool) Close() {
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return
	}
	p.closed = true
	close(p.jobs)
	p.closeMu.Unlock()
	p.wg.Wait()
}

// ErrPoolClosed is returned if a Submit is attempted after Close.
var ErrPoolClosed = &PoolError{"worker pool closed"}

// PoolError provides a simple typed error for pool operations.
type PoolError struct{ msg string }

func (e *PoolError) Error() string { return e.msg }
