package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DBExecutor is an interface that allows methods to accept either *sql.DB or *sql.Tx
type DBExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// isUniqueConstraintErr returns true when the error indicates a unique/constraint violation
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint failed")
}

// CreateOrGetWord returns existing word id or inserts a new word and returns its id.
func CreateOrGetWord(db DBExecutor, word, lemma, reading, definitions, language string) (int64, error) {
	trimmedWord := strings.TrimSpace(word)
	if trimmedWord == "" {
		return 0, fmt.Errorf("word must be non-empty")
	}

	var id int64
	query := `INSERT INTO words (word, lemma, pronunciation, definitions, language) 
			  VALUES (?, ?, ?, ?, ?)
			  ON CONFLICT(word, lemma, language) 
			  DO UPDATE SET 
			    pronunciation = COALESCE(NULLIF(excluded.pronunciation, ''), words.pronunciation),
				definitions = COALESCE(NULLIF(excluded.definitions, ''), words.definitions)
			  RETURNING id`

	err := db.QueryRow(query, trimmedWord, lemma, reading, definitions, language).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert word: %w", err)
	}
	return id, nil
}

// CreateOrGetSource returns existing source id or inserts a new source and returns its id.
func CreateOrGetSource(db DBExecutor, sourceType, title, author, website, url, meta string) (int64, error) {
	trimmedSourceType := strings.TrimSpace(sourceType)
	if trimmedSourceType == "" {
		return 0, fmt.Errorf("sourceType must be non-empty")
	}

	const maxRetries = 3

	var id int64
	for attempt := 0; attempt < maxRetries; attempt++ {
		// First, try to find an existing source.
		err := db.QueryRow(
			`SELECT id FROM sources WHERE IFNULL(url, '') = ? AND IFNULL(title, '') = ? AND IFNULL(author, '') = ?`,
			url, title, author,
		).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return 0, err
		}

		// No existing row; try to insert one.
		res, err := db.Exec(
			`INSERT INTO sources (source_type, title, author, website, url, meta) VALUES (?, ?, ?, ?, ?, ?)`,
			trimmedSourceType, title, author, website, url, meta,
		)
		if err != nil {
			// If another concurrent transaction inserted the same source, retry the SELECT.
			if isUniqueConstraintErr(err) {
				continue
			}
			return 0, err
		}

		// Insert succeeded; return the id directly
		return res.LastInsertId()
	}

	// If we've exhausted all retries, return an error
	return 0, fmt.Errorf("could not create or get source after %d retries", maxRetries)
}

// LinkWordToSource links the word and source, creating or updating an entry in word_sources.
func getOrCreateSentence(db DBExecutor, text string) (int64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, nil
	}
	var id int64
	// Try to find existing sentence first
	if err := db.QueryRow(`SELECT id FROM sentences WHERE text = ?`, trimmed).Scan(&id); err == nil {
		return id, nil
	} else if err != sql.ErrNoRows {
		return 0, err
	}
	// Insert if missing (concurrent-safe via UNIQUE constraint)
	if _, err := db.Exec(`INSERT OR IGNORE INTO sentences (text) VALUES (?)`, trimmed); err != nil {
		return 0, err
	}
	// Select again to get id
	if err := db.QueryRow(`SELECT id FROM sentences WHERE text = ?`, trimmed).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// LinkWordToSource links the word and source, creating or updating an entry in word_sources.
func LinkWordToSource(db DBExecutor, wordID, sourceID int64, context, example string, incrementAmount int) error {
	if wordID <= 0 {
		return fmt.Errorf("wordID must be positive")
	}
	if sourceID <= 0 {
		return fmt.Errorf("sourceID must be positive")
	}
	if incrementAmount < 1 {
		return fmt.Errorf("incrementAmount must be positive, got %d", incrementAmount)
	}

	// Get or create sentences
	ctxID, err := getOrCreateSentence(db, context)
	if err != nil {
		return fmt.Errorf("get/create context sentence: %w", err)
	}
	exID, err := getOrCreateSentence(db, example)
	if err != nil {
		return fmt.Errorf("get/create example sentence: %w", err)
	}

	// Use SQLite UPSERT to atomically insert or update occurrence_count and sentence ids
	var wordSourceID int64
	err = db.QueryRow(`INSERT INTO word_sources (word_id, source_id, context_sentence_id, example_sentence_id, occurrence_count, first_seen_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(word_id, source_id) DO UPDATE SET
	  occurrence_count = word_sources.occurrence_count + excluded.occurrence_count,
	  context_sentence_id = excluded.context_sentence_id,
	  example_sentence_id = excluded.example_sentence_id
	RETURNING id`, wordID, sourceID, nullableInt64(ctxID), nullableInt64(exID), incrementAmount, time.Now()).Scan(&wordSourceID)
	if err != nil {
		return err
	}

	// Limit stored contexts to 5 per word-source pair
	// Atomic insert using INSERT ... SELECT ... WHERE count < 5
	_, err = db.Exec(`
		INSERT INTO word_contexts (word_source_id, sentence_id)
		SELECT ?, ?
		WHERE (SELECT COUNT(*) FROM word_contexts WHERE word_source_id = ?) < 5
		ON CONFLICT DO NOTHING`,
		wordSourceID, nullableInt64(ctxID), wordSourceID)

	return err
}

// nullableInt64 returns nil for 0 (meaning no sentence) else the value.
func nullableInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// UpdateWordDefinitions updates the definitions JSON for a given word.
func UpdateWordDefinitions(db DBExecutor, wordID int64, definitions string) error {
	if wordID <= 0 {
		return fmt.Errorf("wordID must be positive")
	}
	_, err := db.Exec(`UPDATE words SET definitions = ? WHERE id = ?`, definitions, wordID)
	return err
}

// GetWordsBySource returns words associated with a given source id.
func GetWordsBySource(db DBExecutor, sourceID int64) ([]Word, error) {
	rows, err := db.Query(`SELECT w.id, w.word, w.lemma, w.language, w.pronunciation, w.image_url, w.mnemonic_text, w.definitions FROM words w JOIN word_sources ws ON ws.word_id = w.id WHERE ws.source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Word
	for rows.Next() {
		var w Word
		var lemma, lang sql.NullString
		var pron, img, mn sql.NullString
		var defs sql.NullString
		if err := rows.Scan(&w.ID, &w.Word, &lemma, &lang, &pron, &img, &mn, &defs); err != nil {
			return nil, err
		}
		if lemma.Valid {
			w.Lemma = lemma.String
		}
		if lang.Valid {
			w.Language = lang.String
		}
		if pron.Valid {
			w.Pronunciation = pron.String
		}
		if img.Valid {
			w.ImageURL = img.String
		}
		if mn.Valid {
			w.MnemonicText = mn.String
		}
		if defs.Valid {
			w.Definitions = defs.String
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetSourceProgress returns the last processed sentence index for a source.
func GetSourceProgress(db DBExecutor, sourceID int64) (int, error) {
	var index int
	err := db.QueryRow("SELECT last_processed_sentence FROM sources WHERE id = ?", sourceID).Scan(&index)
	if err != nil {
		return 0, err
	}
	return index, nil
}

// UpdateSourceProgress updates the last processed sentence index.
func UpdateSourceProgress(db DBExecutor, sourceID int64, index int) error {
	_, err := db.Exec("UPDATE sources SET last_processed_sentence = ? WHERE id = ?", index, sourceID)
	return err
}
