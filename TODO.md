# TODO

## Phase 1: CLI Reader Pipeline
- [x] Install `github.com/go-shiori/go-readability`
- [x] Create CLI entrypoint (`cmd/readerer/main.go`)
- [x] Connect URL fetcher -> Readability Extractor -> Kagome Tokenizer
- [x] Output list of Lemma + Reading to stdout
- [x] Add HTML and raw-text test fixtures and unit tests for pipeline (`pkg/readerer/testdata/*`, `pkg/readerer/readerer_test.go`)
- [x] Add Mainichi article fixture and regression test for pipeline

## Phase 2: Persistence & Context (Done)
- [x] **Connect CLI to DB**: Initialize SQLite in `main.go`.
- [x] **Persist Source**: Save article metadata (Title, URL, Date) to `sources` table.
- [x] **Context Extraction**: Detect sentence boundaries during tokenization to capture the full sentence for each word.
- [x] **Persist Words & Links**: 
  - [x] Save unique tokens to `words` table.
  - [x] Link words to the source in `word_sources`, including the captured context sentence.
- [x] **Content Filtering**: Filter out punctuation/symbols (`記号`) from being saved as "vocabulary".

## Phase 3: Dictionary & Definitions (Done)
- [x] **Dictionary Import**: Import Jitendex/JMdict JSONs to populate `definitions`.
- [x] **Word Lookup**: Match saved words against imported dictionary entries.
- [x] **Auto-Download & Pipeline Integration**:
  - [x] **Task**: Implement `downloader.go` to fetch JMdict (English) if missing.
  - [x] **Task**: Integrate into CLI pipeline: Ensure dictionary is loaded and definitions are applied when adding new words.
  - [x] Check for cached dictionary locally.
  - [x] If missing, automatically download specific JMdict (English) release.
  - [x] Decompress and cache for future runs.
  - [ ] **Filter Non-Japanese**: Filter out tokens that are non-Japanese (e.g. English words, pure numbers) to ensure only Japanese vocabulary is captured.
  
- [x] **Multi-Context Storage (Done)**:
  - [x] Update schema to store multiple context sentences per word-source pair (instead of overwriting).
  - [x] Limit to top 5 contexts (e.g., store as JSON array or separate table).

## Phase 4: Anki Export (Deferred)
- [ ] **Anki Integration**: Connect to AnkiConnect to push new cards.

## Future enhancements (deferred)
- **Web UI**: Building the frontend (Deferred in favor of CLI for MVP).
- Add similar-word suggestions
  - Option 1 (fast): similarity = same lemma or POS — SELECT other words with same lemma from other sources.
  - Option 2 (advanced): semantic similarity via embedding index (future feature).

- Add semantic search / embeddings for improved discovery and clustering of words across sources.
