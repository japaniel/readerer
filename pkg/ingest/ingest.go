package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"

	"github.com/japaniel/readerer/pkg/db"
	"github.com/japaniel/readerer/pkg/dictionary"
	"github.com/japaniel/readerer/pkg/readerer"
)

// Ingester handles the ingestion of sentences into the database.
type Ingester struct {
	DB           *sql.DB
	DictImporter *dictionary.Importer
	BatchSize    int
	// Logger is used for informational messages (e.g. resume status). nil means no logging.
	Logger *log.Logger
	// OnProgress is called periodically with the number of processed sentences and total sentences.
	OnProgress func(current, total int)
}

// NewIngester creates a new Ingester.
func NewIngester(conn *sql.DB, dict *dictionary.Importer) *Ingester {
	return &Ingester{
		DB:           conn,
		DictImporter: dict,
		BatchSize:    50,
	}
}

// Ingest processes sentences and saves them to the database.
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

	asciiRegex := regexp.MustCompile(`^[a-zA-Z0-9\s[:punct:]]+$`)
	linkCount := 0

	var tx *sql.Tx

	commitTx := func() error {
		if tx == nil {
			return nil
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		tx = nil
		return nil
	}

	// Ensure rollback if panic or error return without commit
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for i, sentence := range sentences {
		// Skip already processed
		if i <= lastProcessed {
			continue
		}

		select {
		case <-ctx.Done():
			// Attempt to commit whatever we have done in this batch so far?
			// Generally safer to rollback incomplete batches, but if we haven't updated progress,
			// the data is there but "untracked".
			// Actually, if we rollback, we lose the work of this partial batch, which is fine (consistency).
			return linkCount, ctx.Err()
		default:
		}

		if tx == nil {
			var err error
			tx, err = ig.DB.Begin()
			if err != nil {
				return linkCount, fmt.Errorf("failed to begin transaction: %w", err)
			}
		}

		cleanSentence := sentence.Text

		// Aggregation maps for the current sentence
		wordCounts := make(map[string]int)
		wordReadings := make(map[string]string)
		var orderedWords []string

		for _, t := range sentence.Tokens {
			// Filtering
			// Skip symbols, particles (助詞), and auxiliary verbs (助動詞)
			// Also skip unknown POS if needed, but basic filtering helps clean up.
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
				// If the existing reading is empty but this token provides a non-empty reading,
				// update the stored reading for this word.
				currentReading := wordReadings[wordToSave]
				newReading := dictionary.ToHiragana(t.Reading)
				if currentReading == "" && newReading != "" {
					wordReadings[wordToSave] = newReading
				}
			}
			wordCounts[wordToSave]++
		}

		// Process words in the order they were first encountered in the sentence
		// (preserve first-seen token order) to ensure deterministic behavior and
		// stable insertion order into the DB. Avoid iterating over `wordCounts`
		// directly because map iteration order is nondeterministic.
		for _, wordToSave := range orderedWords {
			count := wordCounts[wordToSave]
			// Dictionary Lookup logic
			var definitions string
			// Default reading is initialized from the first token occurrence
			readingToSave := wordReadings[wordToSave]

			if ig.DictImporter != nil {
				// Lookup using the canonical word (Lemma).
				matches, _ := ig.DictImporter.Lookup(wordToSave, wordToSave, "")
				if len(matches) > 0 {
					d, err := dictionary.FormatDefinitions(matches)
					if err == nil {
						definitions = d
					}

					// Use the dictionary's primary reading for this Lemma.
					if len(matches[0].Kana) > 0 {
						foundReading := ""
						// Try to find a common reading
						for _, k := range matches[0].Kana {
							if k.Common {
								foundReading = k.Text
								break
							}
						}
						// If no common reading, take the first one
						if foundReading == "" {
							foundReading = matches[0].Kana[0].Text
						}
						readingToSave = dictionary.ToHiragana(foundReading)
					}
				}
			}

			// DB Operations using TX
			// Use BaseForm as primary word to normalize conjugations (e.g. save '書く' instead of '書い')
			// wordToSave is already normalized to BaseForm (or valid Surface if no lemma found).
			wordID, err := db.CreateOrGetWord(tx, wordToSave, wordToSave, readingToSave, definitions, "ja")
			if err != nil {
				if ig.Logger != nil {
					ig.Logger.Printf("Failed to persist word %s: %v", wordToSave, err)
				}
				continue
			}

			// Link with count
			err = db.LinkWordToSource(tx, wordID, sourceID, cleanSentence, cleanSentence, count)
			if err != nil {
				if ig.Logger != nil {
					ig.Logger.Printf("Failed to link word %d: %v", wordID, err)
				}
			} else {
				linkCount += count
			}
		}

		// Checkpoint
		if (i+1)%ig.BatchSize == 0 {
			if err := db.UpdateSourceProgress(tx, sourceID, i); err != nil {
				if ig.Logger != nil {
					ig.Logger.Printf("Warning: failed to save progress: %v", err)
				}
			}
			if err := commitTx(); err != nil {
				return linkCount, err
			}
			if ig.OnProgress != nil {
				ig.OnProgress(i+1, len(sentences))
			}
		}
	}

	// Final commit
	if tx != nil {
		if err := db.UpdateSourceProgress(tx, sourceID, len(sentences)-1); err != nil {
			if ig.Logger != nil {
				ig.Logger.Printf("Warning: failed to update final progress: %v", err)
			}
		}
		if err := commitTx(); err != nil {
			return linkCount, err
		}
	}
	// Explicit final progress update
	if ig.OnProgress != nil {
		ig.OnProgress(len(sentences), len(sentences))
	}

	return linkCount, nil
}
