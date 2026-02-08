package dictionary

import (
	"encoding/json"
	"fmt"
	"os"
)

// JMdictEntry matches the structure of jmdict-simplified entries.
type JMdictEntry struct {
	Id    string          `json:"id"`
	Kanji []JMdictElement `json:"kanji"`
	Kana  []JMdictElement `json:"kana"`
	Sense []JMdictSense   `json:"sense"`
}

type JMdictElement struct {
	Text   string   `json:"text"`
	Common bool     `json:"common"`
	Tags   []string `json:"tags"`
}

type JMdictSense struct {
	PartOfSpeech []string      `json:"partOfSpeech"`
	Gloss        []JMdictGloss `json:"gloss"`
}

type JMdictGloss struct {
	Text string `json:"text"`
	Lang string `json:"lang"` // defaults to 'eng' if missing
}

// DefinitionEntry is what we save to the DB in the 'definitions' column (as JSON list).
type DefinitionEntry struct {
	Senses []string `json:"senses"`
	POS    []string `json:"pos"`
}

// LoadJMdictSimplified reads a JSON file (array of entries) and returns them.
// Note: Real files are large, so in production we might want to stream this.
// For now, we'll load specific chunks or a full file if memory allows.
func LoadJMdictSimplified(path string) ([]JMdictEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var getEntries struct {
		Words []JMdictEntry `json:"words"`
	}
	// Try parsing as full object wrapper first { "words": [...] }
	dec := json.NewDecoder(f)
	if err := dec.Decode(&getEntries); err == nil && len(getEntries.Words) > 0 {
		return getEntries.Words, nil
	}

	// Reset and try as array [...]
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	var entries []JMdictEntry
	dec = json.NewDecoder(f)
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to parse dictionary as object or array: %w", err)
	}
	return entries, nil
}
