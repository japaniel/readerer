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

		for _, t := range sentence.Tokens {
			// Filtering
			// Skip symbols, particles (助詞), and auxiliary verbs (助動詞)
			if t.PrimaryPOS == "記号" || t.PrimaryPOS == "補助記号" || t.PrimaryPOS == "助詞" || t.PrimaryPOS == "助動詞" {
				continue
			}
			if len(t.PartsOfSpeech) > 1 && t.PartsOfSpeech[1] == "数" {
				continue
			}
			if asciiRegex.MatchString(t.Surface) {
				continue
			}

			// Dictionary Lookup logic
			var definitions string
			reading := t.Reading

			if ig.DictImporter != nil {
				matches, _ := ig.DictImporter.Lookup(t.Surface, t.BaseForm, t.Reading)
				if len(matches) > 0 {
					d, err := dictionary.FormatDefinitions(matches)
					if err == nil {
						definitions = d
					}
					targetHiragana := dictionary.ToHiragana(t.Reading)
					foundPreferredReading := false
					for _, k := range matches[0].Kana {
						if k.Text == targetHiragana {
							reading = k.Text
							foundPreferredReading = true
							break
						}
					}
					if !foundPreferredReading {
						reading = targetHiragana
					}
				} else {
					reading = dictionary.ToHiragana(t.Reading)
				}
			}

			// DB Operations using TX
			// Use BaseForm as primary word to normalize conjugations (e.g. save '書く' instead of '書い')
			// t.BaseForm is already normalized to Surface if no lemma is found.
			wordID, err := db.CreateOrGetWord(tx, t.BaseForm, t.BaseForm, reading, definitions, "ja")
			if err != nil {
				if ig.Logger != nil {
					ig.Logger.Printf("Failed to persist word %s: %v", t.BaseForm, err)
				}
				continue
			}

			err = db.LinkWordToSource(tx, wordID, sourceID, cleanSentence, cleanSentence)
			if err != nil {
				if ig.Logger != nil {
					ig.Logger.Printf("Failed to link word %d: %v", wordID, err)
				}
			} else {
				linkCount++
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
