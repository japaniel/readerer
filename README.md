# Readerer

A Go-based tool for Japanese learners to ingest articles, ebooks, and text, extracting unknown vocabulary and creating Anki cards.

## Usage (Planned)

```bash
# Analyze a URL and print tokens
go run ./cmd/readerer -url "https://mariosakata.medium.com/remember-mediumjapan-2f7ce526611c"
```

## Features

- **Article Extraction**: Downloads web pages and isolates the main article text using `go-readability`.
- **Tokenization**: Splits Japanese text into words using `Kagome` (Pure Go MeCab port).
- **Analysis**: Provides Lemma (dictionary form) and Reading (Katakana) for each word.

## Development

Prerequisites:
- Go 1.20+
- (Optional) Dev Container included.
