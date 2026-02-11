package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// WriteFunc is a callback that performs database writes inside a transaction.
type WriteFunc func(ctx context.Context, tx *sql.Tx) error

// BatchWriter buffers write operations and flushes them in batches inside a transaction.
type BatchWriter struct {
	mu          sync.Mutex
	buf         []WriteFunc
	cap         int
	flushTicker *time.Ticker
	closed      bool
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc

	commitCh chan []WriteFunc
	db       *sql.DB
	OnError  func(error)

	// lastErr stores the first asynchronous error seen by the writer. Protected by errMu.
	errMu   sync.Mutex
	lastErr error
}

// NewBatchWriter creates a new BatchWriter.
// db: the database connection to use for transactions.
// bufferSize: flush when buffer reaches this size.
// flushInterval: flush after this duration (0 to disable).
func NewBatchWriter(db *sql.DB, bufferSize int, flushInterval time.Duration) *BatchWriter {
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
		commitCh:    make(chan []WriteFunc, 2), // Buffer a couple of batches
		db:          db,
	}

	bw.wg.Add(1)
	go bw.committer()

	if flushInterval > 0 {
		bw.flushTicker = time.NewTicker(flushInterval)
		bw.wg.Add(1)
		go bw.loop()
	}
	return bw
}

// Submit enqueues a write function.
func (bw *BatchWriter) Submit(w WriteFunc) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	if bw.closed {
		return ErrBatchWriterClosed
	}
	bw.buf = append(bw.buf, w)
	if len(bw.buf) >= bw.cap {
		bw.flushLocked()
	}
	return nil
}

// flushLocked assumes bw.mu is held.
func (bw *BatchWriter) flushLocked() {
	if len(bw.buf) == 0 {
		return
	}
	batch := bw.buf
	bw.buf = make([]WriteFunc, 0, bw.cap)

	// Send to committer.
	// Note: We cannot block indefinitely here while holding the lock,
	// because Submit() calls this. If committer is stuck, Submit blocks, which propagates backpressure.
	// However, Close() also calls this under lock.
	select {
	case bw.commitCh <- batch:
	case <-bw.ctx.Done():
		// shutdown: report dropped batch via OnError and record the error so callers can detect potential data loss.
		err := fmt.Errorf("batch writer: dropping batch of %d items due to context cancellation", len(batch))
		bw.errMu.Lock()
		if bw.lastErr == nil {
			bw.lastErr = err
		}
		bw.errMu.Unlock()
		if bw.OnError != nil {
			bw.OnError(err)
		}
	}

}

func (bw *BatchWriter) committer() {
	defer bw.wg.Done()
	for batch := range bw.commitCh {
		if err := bw.executeBatch(batch); err != nil {
			// Persist the first async error so callers can retrieve it after Close().
			bw.errMu.Lock()
			if bw.lastErr == nil {
				bw.lastErr = err
			}
			bw.errMu.Unlock()
			if bw.OnError != nil {
				bw.OnError(err)
			}
		}
	}
}

func (bw *BatchWriter) executeBatch(batch []WriteFunc) error {
	// If no DB is configured (e.g. testing without DB), just run callbacks with nil tx
	if bw.db == nil {
		for _, w := range batch {
			if err := w(bw.ctx, nil); err != nil {
				return err
			}
		}
		return nil
	}

	// Use background context for flushing to avoid "context canceled" if bw is closing.
	ctx := context.Background()

	tx, err := bw.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin batch tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // ignored if committed
	}()

	for _, w := range batch {
		if err := w(ctx, tx); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch (%d items): %w", len(batch), err)
	}
	return nil
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

	bw.cancel()        // Stop ticker loop
	close(bw.commitCh) // Stop committer loop
	bw.wg.Wait()

	// Return any async error that was recorded during execution
	bw.errMu.Lock()
	defer bw.errMu.Unlock()
	if bw.lastErr != nil {
		return bw.lastErr
	}
	return nil
}

var ErrBatchWriterClosed = &BatchWriterError{"batch writer closed"}

type BatchWriterError struct{ msg string }

func (e *BatchWriterError) Error() string { return e.msg }
