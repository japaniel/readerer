package db

import (
	"database/sql"
	"time"
)

// CreateOrGetWord returns existing word id or inserts a new word and returns its id.
func CreateOrGetWord(db *sql.DB, word, lemma, language string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM words WHERE word = ? AND IFNULL(lemma, '') = ? AND IFNULL(language, '') = ?`, word, lemma, language).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		// insert
		res, err := db.Exec(`INSERT INTO words (word, lemma, language) VALUES (?, ?, ?)`, word, lemma, language)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	// Not found path: insert
	res, err := db.Exec(`INSERT INTO words (word, lemma, language) VALUES (?, ?, ?)`, word, lemma, language)
	if err != nil {
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
	res, err := db.Exec(`INSERT INTO sources (source_type, title, author, website, url, meta) VALUES (?, ?, ?, ?, ?, ?)`, sourceType, title, author, website, url, meta)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LinkWordToSource links the word and source, creating or updating an entry in word_sources.
func LinkWordToSource(db *sql.DB, wordID, sourceID int64, context, example string) error {
	// try update
	res, err := db.Exec(`UPDATE word_sources SET occurrence_count = occurrence_count + 1 WHERE word_id = ? AND source_id = ?`, wordID, sourceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	_, err = db.Exec(`INSERT INTO word_sources (word_id, source_id, context_sentence, example_sentence, occurrence_count, first_seen_at) VALUES (?, ?, ?, ?, 1, ?)`, wordID, sourceID, context, example, time.Now())
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
