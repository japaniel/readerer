package dictionary

import (
	"database/sql"
	"encoding/json"
	"log"
	"sort"
	"sync"

	"github.com/japaniel/readerer/pkg/db"
)

// Importer handles dictionary matching and updating.
type Importer struct {
	conn *sql.DB
	// Maps to speed up lookups.
	// Key: string (Kanji or Kana), Value: List of matching JMdictEntry
	// Note: `index` is read concurrently by multiple goroutines; guard reads with `mu` to
	// protect against future code that might mutate the map. If the index is never
	// mutated after creation this is a no-op, but the mutex provides safety for later changes.
	mu    sync.RWMutex
	index map[string][]JMdictEntry
}

// NewImporter creates an importer and builds an in-memory index of the provided dictionary.
func NewImporter(conn *sql.DB, entries []JMdictEntry) *Importer {
	idx := make(map[string][]JMdictEntry)
	for _, e := range entries {
		// Index by Kanji
		for _, k := range e.Kanji {
			idx[k.Text] = append(idx[k.Text], e)
		}
		// Index by Kana
		for _, k := range e.Kana {
			idx[k.Text] = append(idx[k.Text], e)
		}
	}
	return &Importer{
		conn:  conn,
		index: idx,
	}
}

// ProcessUpdates finds definitions for words in the DB and updates them.
func (im *Importer) ProcessUpdates() (int, error) {
	// 1. Fetch all words
	rows, err := im.conn.Query(`SELECT id, word, lemma, pronunciation, definitions FROM words`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	updatedCount := 0

	// We'll collect updates and apply them to avoid locking issues if possible,
	// though SQLite handles single logic connection fine.
	type update struct {
		id  int64
		def string
	}
	var updates []update

	for rows.Next() {
		var id int64
		var word string
		var lemma, pronunciation, definitions sql.NullString

		if err := rows.Scan(&id, &word, &lemma, &pronunciation, &definitions); err != nil {
			return updatedCount, err
		}

		// Skip if already has definitions (optional: force update flag?)
		if definitions.Valid && definitions.String != "" {
			continue
		}

		// Lookup
		matchedEntries := im.findMatches(word, lemma.String, pronunciation.String)
		if len(matchedEntries) == 0 {
			continue
		}

		// Convert to stored JSON format
		defJSON, err := FormatDefinitions(matchedEntries)
		if err != nil {
			log.Printf("Error formatting definition for word %s: %v", word, err)
			continue
		}

		updates = append(updates, update{id, defJSON})
	}

	// Apply updates
	for _, u := range updates {
		if err := db.UpdateWordDefinitions(im.conn, u.id, u.def); err != nil {
			log.Printf("Failed to update word %d: %v", u.id, err)
		} else {
			updatedCount++
		}
	}

	return updatedCount, nil
}

// Lookup finds matching entries for a given word, lemma, and pronunciation.
func (im *Importer) Lookup(word, lemma, pronunciation string) ([]JMdictEntry, error) {
	matches := im.findMatches(word, lemma, pronunciation)
	if len(matches) == 0 {
		return nil, nil // or error "not found"
	}
	return matches, nil
}

// GetDefinitionsJSON returns the JSON string of definitions for the given word details.
func (im *Importer) GetDefinitionsJSON(word, lemma, pronunciation string) (string, error) {
	matches := im.findMatches(word, lemma, pronunciation)
	if len(matches) == 0 {
		return "", nil
	}
	return FormatDefinitions(matches)
}

func (im *Importer) findMatches(word, lemma, pronunciation string) []JMdictEntry {
	// Strategy:
	// 1. Try exact match on 'word' (Surface)
	// 2. Try match on 'lemma' (BaseForm)
	// 3. Filter results by pronunciation if available

	candidates := make(map[string]JMdictEntry) // use map to dedupe by Entry ID

	// Helper to add candidates
	search := func(term string) {
		if term == "" {
			return
		}
		im.mu.RLock()
		entries, ok := im.index[term]
		im.mu.RUnlock()
		if ok {
			for _, e := range entries {
				candidates[e.Id] = e
			}
		}
	}

	search(word)
	search(lemma)

	// If we have candidates, verify/rank them
	var results []JMdictEntry
	for _, entry := range candidates {
		if isMatch(entry, word, lemma, pronunciation) {
			results = append(results, entry)
		}
	}

	// Sort results deterministically to ensure consistent behavior.
	// Primary: Entry ID.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Id < results[j].Id
	})

	return results
}

func isMatch(entry JMdictEntry, word, lemma, pronunciation string) bool {
	// A match is good if the entry contains the Kanji (word/lemma) AND the Kana (pronunciation).
	// If pronunciation is empty in DB, lax match on text.

	hasText := false
	for _, k := range entry.Kanji {
		if k.Text == word || k.Text == lemma {
			hasText = true
			break
		}
	}
	// Also check Kana elements for text match (words usually written in Kana)
	for _, k := range entry.Kana {
		if k.Text == word || k.Text == lemma {
			hasText = true
			break
		}
	}
	if !hasText {
		return false
	}

	if pronunciation == "" {
		return true
	}

	normalizedPron := ToHiragana(pronunciation)

	// Verify reading
	// If entry has restricted reading (kanji entry has specific reading), it's complex.
	// Simple check: does any generic kana match the pronunciation?
	hasReading := false
	for _, k := range entry.Kana {
		if ToHiragana(k.Text) == normalizedPron {
			hasReading = true
			break
		}
	}

	return hasReading
}

// ToHiragana converts Katakana to Hiragana.
func ToHiragana(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if r >= 0x30A1 && r <= 0x30F6 {
			runes[i] = r - 0x60
		}
	}
	return string(runes)
}

// FormatDefinitions formats the entries into a JSON string.
func FormatDefinitions(entries []JMdictEntry) (string, error) {
	// Combine senses from multiple matching entries if necessary, or just take the first/best.
	// Flatten to a simple list of glosses + POS
	var defs []DefinitionEntry

	for _, e := range entries {
		var senses []string
		var poses []string

		for _, s := range e.Sense {
			// Extract glosses
			for _, g := range s.Gloss {
				senses = append(senses, g.Text)
			}
			// Extract POS (just accum unique ones?)
			for _, p := range s.PartOfSpeech {
				poses = append(poses, p)
			}
		}
		defs = append(defs, DefinitionEntry{
			Senses: senses,
			POS:    poses,
		})
	}

	bytes, err := json.Marshal(defs)
	return string(bytes), err
}
