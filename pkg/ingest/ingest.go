package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/dictionary"
	"github.com/japaniel/readerer/pkg/readerer"
)

// WorkerPoolInterface abstracts the worker pool so tests can inject failing implementations.
type WorkerPoolInterface interface {
	Start(ctx context.Context)
	Submit(Job) error
	// SubmitCtx attempts to enqueue a job but returns promptly if ctx is canceled.
	SubmitCtx(ctx context.Context, job Job) error
	Close()
}

// Ingester handles the ingestion of sentences into the database.
type Ingester struct {
	DB           *sql.DB
	DictImporter *dictionary.Importer
	BatchSize    int
	// Logger is used for informational messages (e.g. resume status). nil means no logging.
	Logger *log.Logger
	// OnProgress is called periodically with the number of processed sentences and total sentences.
	OnProgress func(current, total int)

	// Concurrency settings
	Workers int

	// PoolFactory allows tests to inject custom worker pool implementations.
	PoolFactory func(workers, queue int) WorkerPoolInterface
}

// NewIngester creates a new Ingester.
func NewIngester(conn *sql.DB, dict *dictionary.Importer) *Ingester {
	return &Ingester{
		DB:           conn,
		DictImporter: dict,
		BatchSize:    50,
		Workers:      4, // Default worker count
	}
}

// wordData holds prepared data for a single word occurrence in a sentence
type wordData struct {
	Word        string
	Reading     string
	Definitions string
	Count       int
}

// processedSentence holds the result of processing a sentence before DB ingestion
type processedSentence struct {
	Index    int
	Sentence string
	Words    []wordData
	Error    error
}

// Ingest processes sentences and saves them to the database using concurrent workers and batched writes.
// It supports resuming from the last checkpoint using the sourceID.
func (ig *Ingester) Ingest(ctx context.Context, sourceID int64, sentences []readerer.Sentence) (int, error) {
	// Check progress
	lastProcessed, err := db.GetSourceProgress(ig.DB, sourceID)
	if err != nil {
		if ig.Logger != nil {
			ig.Logger.Printf("Warning: Failed to retrieve progress: %v", err)
		}
		lastProcessed = -1
	}

	if lastProcessed >= 0 {
		if ig.Logger != nil {
			ig.Logger.Printf("Resuming from sentence index %d (skipping %d messages)\n", lastProcessed+1, lastProcessed+1)
		}
	} else if lastProcessed == -1 {
		// Just starting or no progress found
	}

	totalSentences := len(sentences)
	startIdx := lastProcessed + 1
	if startIdx >= totalSentences {
		return 0, nil // Nothing to do
	}

	// 1. Setup concurrency components
	var wp WorkerPoolInterface
	if ig.PoolFactory != nil {
		wp = ig.PoolFactory(ig.Workers, ig.Workers*2)
	} else {
		wp = NewWorkerPool(ig.Workers, ig.Workers*2)
	}
	resultCh := make(chan processedSentence, ig.Workers*2)
	closedResultCh := false

	// We use a separate channel to communicate final done/error state
	doneCh := make(chan error, 1)

	// Link tracker
	var totalLinks int64

	// BatchWriter for DB operations
	// Flush every BatchSize or 1 second to ensure progress
	bw := NewBatchWriter(ig.DB, ig.BatchSize, 100*time.Millisecond)
	// Capture first error seen in batch writer
	var batchErr error
	var batchErrMu sync.Mutex
	bw.OnError = func(e error) {
		batchErrMu.Lock()
		if batchErr == nil {
			batchErr = e
		}
		batchErrMu.Unlock()
	}

	// Ensure resources are cleaned up on any return path: stop workers, close resultCh, flush batches.
	defer func() {
		// Close the worker pool and other resources on exit.
		wp.Close()
		if !closedResultCh {
			close(resultCh)
		}
		// Best-effort close; ignore already-closed errors
		_ = bw.Close()
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wp.Start(ctx)

	go func() {
		defer close(doneCh)
		buffer := make(map[int]processedSentence)
		nextIdx := startIdx

		for {
			select {
			case <-ctx.Done():
				doneCh <- ctx.Err()
				return
			default:
			}

			res, ok := <-resultCh
			if !ok {
				// resultCh closed; process any remaining contiguous entries then exit
				for {
					item, ok := buffer[nextIdx]
					if !ok {
						break
					}
					delete(buffer, nextIdx)

					currentItem := item
					err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
						for _, w := range currentItem.Words {
							wordID, err := db.CreateOrGetWord(tx, w.Word, w.Word, w.Reading, w.Definitions, "ja")
							if err != nil {
								return fmt.Errorf("failed to persist word %s: %w", w.Word, err)
							}
							if err := db.LinkWordToSource(tx, wordID, sourceID, currentItem.Sentence, currentItem.Sentence, w.Count); err != nil {
								return fmt.Errorf("failed to link word %d: %w", wordID, err)
							}
							atomic.AddInt64(&totalLinks, int64(w.Count))
						}
						if err := db.UpdateSourceProgress(tx, sourceID, currentItem.Index); err != nil {
							return fmt.Errorf("failed to save progress: %w", err)
						}
						return nil
					})

					if err != nil {
						// Signal producers to stop to prevent them from blocking on resultCh.
						cancel()
						return
					}

					if ig.OnProgress != nil && (nextIdx+1)%ig.BatchSize == 0 {
						ig.OnProgress(nextIdx+1, totalSentences)
					}
					nextIdx++
				}

				if ig.OnProgress != nil {
					ig.OnProgress(totalSentences, totalSentences)
				}
				doneCh <- nil
				return
			}

			if res.Error != nil {
				fmt.Println("consumer: got res.Error", res.Error)
				// Ensure producers are signaled to stop so they don't block writing to resultCh.
				cancel()
				doneCh <- res.Error
				return
			}
			buffer[res.Index] = res

			// Process contiguous finished items
			for {
				item, ok := buffer[nextIdx]
				if !ok {
					break
				}
				delete(buffer, nextIdx)

				// Submit DB write job to BatchWriter
				// Isolate loop variable
				currentItem := item
				err := bw.Submit(func(ctx context.Context, tx *sql.Tx) error {
					for _, w := range currentItem.Words {
						wordID, err := db.CreateOrGetWord(tx, w.Word, w.Word, w.Reading, w.Definitions, "ja")
						if err != nil {
							return fmt.Errorf("failed to persist word %s: %w", w.Word, err)
						}
						if err := db.LinkWordToSource(tx, wordID, sourceID, currentItem.Sentence, currentItem.Sentence, w.Count); err != nil {
							return fmt.Errorf("failed to link word %d: %w", wordID, err)
						}
						atomic.AddInt64(&totalLinks, int64(w.Count))
					}
					// Checkpoint progress for this sentence
					if err := db.UpdateSourceProgress(tx, sourceID, currentItem.Index); err != nil {
						return fmt.Errorf("failed to save progress: %w", err)
					}
					return nil
				})

				if err != nil {
					// Signal producers to stop to prevent them from blocking on resultCh.
					cancel()
					doneCh <- err
					return
				}

				// Update UI progress (approximate, since batch might not be flushed yet)
				if ig.OnProgress != nil && (nextIdx+1)%ig.BatchSize == 0 {
					ig.OnProgress(nextIdx+1, totalSentences)
				}
				nextIdx++
			}
		}
	}()

	// 3. Producer loop: Submit tokenization jobs
	// The original regex was compiled once
	asciiRegex := regexp.MustCompile(`^[a-zA-Z0-9\s[:punct:]]+$`)

Loop:
	for i := startIdx; i < totalSentences; i++ {
		// handle early exit if consumer failed
		select {
		case <-ctx.Done():
			break Loop
		default:
		}

		idx := i
		sent := sentences[i]

		job := func(ctx context.Context) error {
			// CPU-bound work: Analyze sentence and prepare data
			res := ig.processSentence(idx, sent, asciiRegex)
			fmt.Println("job: processed", idx)

			// Attempt to send result; the channel may be closed if cancellation occurred,
			// so use recover to avoid a send-on-closed-channel panic.
			defer func() {
				if r := recover(); r != nil {
					// swallow send on closed channel panic when shutdown races occur
				}
			}()
			select {
			case resultCh <- res:
			case <-ctx.Done():
			}
			return nil
		}

		// Submit job to the worker pool but remain responsive to context cancellation.
		if err := wp.SubmitCtx(ctx, job); err != nil {
			// If the error is context cancellation, propagate it.
			if err == ctx.Err() {
				break Loop
			}
			// If pool is closed as part of shutdown, break out gracefully.
			if err == ErrPoolClosed {
				break Loop
			}
			return 0, err
		}

	}

	// Ensure there are no more worker goroutines running and close the result channel to
	// signal the consumer that no more items will arrive.
	wp.Close()
	if !closedResultCh {
		close(resultCh)
		closedResultCh = true
	}

	// Wait for consumer to finish processing all results or error out
	consumerErr := <-doneCh

	// BatchWriter will be closed in the deferred cleanup; call here to capture errors early.
	if err := bw.Close(); err != nil {
		if consumerErr == nil {
			consumerErr = err
		}
	}

	// Check for errors occurred during async flush
	batchErrMu.Lock()
	if batchErr != nil && consumerErr == nil {
		consumerErr = batchErr
	}
	batchErrMu.Unlock()

	// Return the accumulated number of linked word occurrences recorded during ingestion.
	// `totalLinks` is updated atomically by DB write callbacks.
	return int(atomic.LoadInt64(&totalLinks)), consumerErr
}

// processSentence performs the CPU-heavy token analysis and dictionary lookup
func (ig *Ingester) processSentence(index int, sentence readerer.Sentence, asciiRegex *regexp.Regexp) processedSentence {
	cleanSentence := sentence.Text
	wordCounts := make(map[string]int)
	wordReadings := make(map[string]string)
	var orderedWords []string

	for _, t := range sentence.Tokens {
		// Filtering
		if t.PrimaryPOS == "記号" || t.PrimaryPOS == "補助記号" || t.PrimaryPOS == "助詞" || t.PrimaryPOS == "助動詞" {
			continue
		}
		if len(t.PartsOfSpeech) > 1 && t.PartsOfSpeech[1] == "数" {
			continue
		}
		if asciiRegex.MatchString(t.Surface) {
			continue
		}

		// Normalization: Use BaseForm (Lemma) as the canonical word if available
		wordToSave := t.Surface
		if t.BaseForm != "" && t.BaseForm != "*" {
			wordToSave = t.BaseForm
		}

		if _, exists := wordCounts[wordToSave]; !exists {
			wordCounts[wordToSave] = 0
			wordReadings[wordToSave] = dictionary.ToHiragana(t.Reading)
			orderedWords = append(orderedWords, wordToSave)
		} else {
			currentReading := wordReadings[wordToSave]
			newReading := dictionary.ToHiragana(t.Reading)
			if currentReading == "" && newReading != "" {
				wordReadings[wordToSave] = newReading
			}
		}
		wordCounts[wordToSave]++
	}

	var words []wordData
	for _, wordToSave := range orderedWords {
		count := wordCounts[wordToSave]
		definitions := ""
		readingToSave := wordReadings[wordToSave]

		if ig.DictImporter != nil {
			matches, _ := ig.DictImporter.Lookup(wordToSave, wordToSave, "")
			if len(matches) > 0 {
				if d, err := dictionary.FormatDefinitions(matches); err == nil {
					definitions = d
				}
				// Use the dictionary's primary reading for this Lemma.
				if len(matches[0].Kana) > 0 {
					foundReading := ""
					for _, k := range matches[0].Kana {
						if k.Common {
							foundReading = k.Text
							break
						}
					}
					if foundReading == "" {
						foundReading = matches[0].Kana[0].Text
					}
					readingToSave = dictionary.ToHiragana(foundReading)
				}
			}
		}
		words = append(words, wordData{
			Word:        wordToSave,
			Reading:     readingToSave,
			Definitions: definitions,
			Count:       count,
		})
	}

	return processedSentence{
		Index:    index,
		Sentence: cleanSentence,
		Words:    words,
	}
}
