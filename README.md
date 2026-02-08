# Readerer

A Go-based tool for Japanese learners to ingest articles, ebooks, and text, extracting unknown vocabulary and creating Anki cards.

## Usage

```bash
# Analyze a URL and ingest into local SQLite database
go run ./cmd/readerer -url "https://mariosakata.medium.com/remember-mediumjapan-2f7ce526611c"
```

The tool will create `readerer.db` in the current directory and:
1.  Download and clean the article content.
2.  Tokenize the Japanese text.
3.  Look up definitions in the built-in dictionary (JMdict).
4.  Save words, definitions, and **context sentences** to the database.

If interrupted, running the command again will **resume** from where it left off.

## Features

- **Article Extraction**: Downloads web pages and isolates the main article text using `go-readability`.
- **Tokenization**: Splits Japanese text into words using `Kagome` (Pure Go MeCab port).
- **Dictionary Lookups**: Automatically downloads and caches the standard [JMdict](https://www.edrdg.org/jmdict/j_jmdict.html) dictionary to provide definitions.
- **Persistence**: Saves vocabulary to a SQLite database (`readerer.db`).
- **Context Awareness**: Captures the sentence where a word was found (up to 5 unique contexts per word).
- **Resumability**: Tracks progress per article so you can restart interrupted ingestions without re-processing everything.
- **Robustness**: Atomic database transactions and memory-safe file handling.

## Development

Prerequisites:
- Go 1.24+
- (Optional) Dev Container included.
