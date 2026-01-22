package db

import (
	"database/sql"
	"strings"
	"time"
)

// isUniqueConstraintErr returns true when the error indicates a unique/constraint violation
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint failed")
}

// CreateOrGetWord returns existing word id or inserts a new word and returns its id.
func CreateOrGetWord(db *sql.DB, word, lemma, language string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM words WHERE word = ? AND IFNULL(lemma, '') = ? AND IFNULL(language, '') = ?`, word, lemma, language).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		// real DB error
		return 0, err
	}

	// Not found -> try to insert. If a race occurs (unique constraint), attempt to select again.
	res, err := db.Exec(`INSERT INTO words (word, lemma, language) VALUES (?, ?, ?)`, word, lemma, language)
	if err != nil {
		if isUniqueConstraintErr(err) {
			// someone else inserted concurrently, try to select again
			return CreateOrGetWord(db, word, lemma, language)
		}
		return 0, err
	}
	return res.LastInsertId()
}

// CreateOrGetSource returns existing source id or inserts a new source and returns its id.
func CreateOrGetSource(db *sql.DB, sourceType, title, author, website, url, meta string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM sources WHERE IFNULL(url, '') = ? AND IFNULL(title, '') = ? AND IFNULL(author, '') = ?`, url, title, author).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	// Use INSERT OR IGNORE to avoid duplicate rows under concurrency, then select the id.
	_, err = db.Exec(`INSERT OR IGNORE INTO sources (source_type, title, author, website, url, meta) VALUES (?, ?, ?, ?, ?, ?)`, sourceType, title, author, website, url, meta)
	if err != nil {
		return 0, err
	}
	// Now the row should exist (either inserted or pre-existing). Select it.
	err = db.QueryRow(`SELECT id FROM sources WHERE IFNULL(url, '') = ? AND IFNULL(title, '') = ? AND IFNULL(author, '') = ?`, url, title, author).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// LinkWordToSource links the word and source, creating or updating an entry in word_sources.
func LinkWordToSource(db *sql.DB, wordID, sourceID int64, context, example string) error {
	// Use SQLite UPSERT to atomically insert or update occurrence_count and context/example
	_, err := db.Exec(`INSERT INTO word_sources (word_id, source_id, context_sentence, example_sentence, occurrence_count, first_seen_at)
	VALUES (?, ?, ?, ?, 1, ?)
	ON CONFLICT(word_id, source_id) DO UPDATE SET
	  occurrence_count = word_sources.occurrence_count + 1,
	  context_sentence = excluded.context_sentence,
	  example_sentence = excluded.example_sentence`, wordID, sourceID, context, example, time.Now())
	return err
}

// GetWordsBySource returns words associated with a given source id.
func GetWordsBySource(db *sql.DB, sourceID int64) ([]Word, error) {
	rows, err := db.Query(`SELECT w.id, w.word, w.lemma, w.language, w.pronunciation, w.image_url, w.mnemonic_text FROM words w JOIN word_sources ws ON ws.word_id = w.id WHERE ws.source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Word
	for rows.Next() {
		var w Word
		var pron, img, mn sql.NullString
		if err := rows.Scan(&w.ID, &w.Word, &w.Lemma, &w.Language, &pron, &img, &mn); err != nil {
			return nil, err
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
		out = append(out, w)
	}
	return out, nil
}
