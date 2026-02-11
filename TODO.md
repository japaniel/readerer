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
  - [x] **Filter Non-Japanese**: Filter out tokens that are non-Japanese (e.g. English words, pure numbers) to ensure only Japanese vocabulary is captured.

## Phase 4: Reliability & Optimization (Done)
- [x] **Resume Ingestion**: Implement checkpointing to resume processing from the last sentence if interrupted.
- [x] **Memory Safety**: Limit reading of large files to prevent OOM.
- [x] **Thread Safety**: Use atomic SQL operations and `ensureColumnExists` for robust DB interactions.
- [x] **Refactoring**: Decouple library logic from CLI UI (logging and progress callbacks).
  
- [x] **Multi-Context Storage (Done)**:
  - [x] Update schema to store multiple context sentences per word-source pair (instead of overwriting).
  - [x] Limit to top 5 contexts (e.g., store as JSON array or separate table).

## Phase 5: Refinement & UI
- [x] **Fix POS Filtering and Normalization**:
  - [x] Update `Ingester` to skip `助詞` (Particles) and `助動詞` (Aux verbs).
  - [x] Store **Lemma/Base Form** as the primary `word` entry instead of Surface form (e.g. store `書く` not `書い`).
  - [x] Consolidate conjugated forms under the single Lemma entry.
- [x] **Fix Duplicate Contexts**: Ensure we don't keep the same context sentence if a word shows up twice in the same sentence.
- [x] **Offline Tests**: Update tests to use local `testdata` content instead of fetching live URLs.
- [x] **Concurrent Processing**: Add concurrency to improve ingestion speed.
  - [x] Design a **Worker Pool** to parallelize tokenization/lookup per-sentence or per-chunk.  
    - [x] Create a `WorkerPool` type with a configurable worker count and job queue.  
    - [x] Ensure deterministic ordering where needed (e.g., per-source checkpoints).
    - [x] Add unit tests and a small benchmark.
  - [x] Implement a **Batch Writer** for SQLite to group DB writes and reduce transaction overhead.  
    - [x] Create a `BatchWriter` type that accepts write callbacks, batches them by size or time, and commits in transactions.  
    - [x] Ensure thread-safety and graceful shutdown/flush on context cancellation.
    - [x] Add tests verifying batching and flush behavior.
  - [x] Integration tasks
    - [x] Refactor `Ingester.Ingest` to submit work to the `WorkerPool` and use the `BatchWriter` for DB writes.
    - [x] Add integration tests (small article fixtures) to validate correctness and throughput improvements.
    - [x] Add metrics and a benchmark to measure improvements.
  
  **Status:** Concurrency implemented with Producer-Consumer pattern. `BatchWriter` uses serialized background flushing for SQLite safety.
- [ ] **Web UI**: Create a web interface to view words (using Meteor).
  - [ ] Create Go API server.
  - [ ] Create Meteor frontend.

## Phase 6: Anki Export (Deferred)
- [ ] **Anki Integration**: Connect to AnkiConnect to push new cards.

## Phase 7: Ebook Support (Kobo/EPUB)
- [ ] **Local File Ingestion**: Add CLI support for ingesting local files (`-file path/to/book.epub`).
- [ ] **EPUB Parsing**:
  - [ ] Research Go libraries for EPUB parsing (e.g. `go-epub` or plain `archive/zip`).
  - [ ] Implement metadata extraction (Title, Author) for the `sources` table.
  - [ ] Implement spine iteration to process chapters in correct order.
- [ ] **Kobo Specifics**:
  - [ ] Handle `.kepub.epub` format quirks.
  - [ ] Strip specific Kobo span tags and styling to ensure clean text extraction.
- [ ] *Note: This implementation will strictly support DRM-free or pre-decrypted files.*

## Future enhancements (deferred)
- **Web UI**: Building the frontend (Deferred in favor of CLI for MVP).
- Add similar-word suggestions
  - Option 1 (fast): similarity = same lemma or POS — SELECT other words with same lemma from other sources.
  - Option 2 (advanced): semantic similarity via embedding index (future feature).

- Add semantic search / embeddings for improved discovery and clustering of words across sources.
