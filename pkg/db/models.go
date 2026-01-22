package db

import "time"

// Word is the canonical word entry.
type Word struct {
	ID            int64
	Word          string
	Lemma         string
	Language      string
	Pronunciation string
	ImageURL      string
	MnemonicText  string
}

// Source is a provenance record for where a word was seen.
type Source struct {
	ID         int64
	SourceType string
	Title      string
	Author     string
	Website    string
	URL        string
	Meta       string
	AddedAt    time.Time
}

// WordSource links a Word with a Source and holds contextual metadata.
type WordSource struct {
	ID              int64
	WordID          int64
	SourceID        int64
	ContextSentence string
	ExampleSentence string
	OccurrenceCount int
	FirstSeenAt     time.Time
	IsPrimary       bool
}
