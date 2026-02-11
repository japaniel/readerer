package ingest

import (
	"context"
	"sync"
	"time"
)

// WriteFunc is a callback that performs database writes inside a transaction-like
// context. The BatchWriter is responsible for calling WriteFuncs in a batch and
// ensuring they run sequentially inside a commit.
type WriteFunc func(ctx context.Context) error

// BatchWriter buffers write operations and flushes them either when a batch
// reaches capacity or when a flush interval elapses.
//
// NOTE: This is a scaffold. The concrete implementation should accept a DB
// executor (or begin/commit helpers). For testing we use in-memory callbacks.
type BatchWriter struct {
	mu          sync.Mutex
	buf         []WriteFunc
	cap         int
	flushTicker *time.Ticker
	closed      bool
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewBatchWriter creates a new BatchWriter. flushInterval=0 disables periodic flushes.
func NewBatchWriter(bufferSize int, flushInterval time.Duration) *BatchWriter {
	if bufferSize <= 0 {
		bufferSize = 10
	}
	ctx, cancel := context.WithCancel(context.Background())
	bw := &BatchWriter{
		buf:         make([]WriteFunc, 0, bufferSize),
		cap:         bufferSize,
		flushTicker: nil,
		ctx:         ctx,
		cancel:      cancel,
	}
	if flushInterval > 0 {
		bw.flushTicker = time.NewTicker(flushInterval)
		bw.wg.Add(1)
		go bw.loop()
	}
	return bw
}

// Submit enqueues a write function for later batched execution. It returns an error if closed.
func (bw *BatchWriter) Submit(w WriteFunc) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	if bw.closed {
		return ErrBatchWriterClosed
	}
	bw.buf = append(bw.buf, w)
	if len(bw.buf) >= bw.cap {
		// trigger a flush asynchronously
		bw.flushLocked()
	}
	return nil
}

// flushLocked assumes bw.mu is held.
func (bw *BatchWriter) flushLocked() {
	batch := bw.buf
	bw.buf = make([]WriteFunc, 0, bw.cap)
	bw.wg.Add(1)
	go func(batch []WriteFunc) {
		defer bw.wg.Done()
		// In a real implementation, begin a transaction and execute each WriteFunc
		// sequentially; commit/rollback on errors. Here we run callbacks to keep
		// behavior testable without a DB dependency.
		for _, w := range batch {
			_ = w(bw.ctx)
		}
	}(batch)
}

func (bw *BatchWriter) loop() {
	defer bw.wg.Done()
	for {
		select {
		case <-bw.ctx.Done():
			return
		case <-bw.flushTicker.C:
			bw.mu.Lock()
			if len(bw.buf) > 0 {
				bw.flushLocked()
			}
			bw.mu.Unlock()
		}
	}
}

// Close stops accepting submissions and waits for pending writes to complete.
func (bw *BatchWriter) Close() error {
	bw.mu.Lock()
	if bw.closed {
		bw.mu.Unlock()
		return ErrBatchWriterClosed
	}
	bw.closed = true
	if bw.flushTicker != nil {
		bw.flushTicker.Stop()
	}
	// flush remaining
	if len(bw.buf) > 0 {
		bw.flushLocked()
	}
	bw.mu.Unlock()
	bw.cancel()
	bw.wg.Wait()
	return nil
}

var ErrBatchWriterClosed = &BatchWriterError{"batch writer closed"}

type BatchWriterError struct{ msg string }

func (e *BatchWriterError) Error() string { return e.msg }
