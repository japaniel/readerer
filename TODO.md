# TODO

## Current Focus: CLI Reader Pipeline
- [ ] Install `github.com/go-shiori/go-readability`
- [ ] Create CLI entrypoint (`cmd/readerer/main.go`)
- [ ] Connect URL fetcher -> Readability Extractor -> Kagome Tokenizer
- [ ] Output list of Lemma + Reading to stdout

## Future enhancements (deferred)

- **Dictionary Import**: Importing Jitendex/JMdict JSONs (Deferred until pipeline works).
- **Anki Integration**: Connecting to AnkiConnect (Deferred until we have words to send).
- **Web UI**: Building the frontend (Deferred in favor of CLI for MVP).
- Add similar-word suggestions
  - Option 1 (fast): similarity = same lemma or POS — SELECT other words with same lemma from other sources.
  - Option 2 (advanced): semantic similarity via embedding index (future feature).

- Add semantic search / embeddings for improved discovery and clustering of words across sources.
