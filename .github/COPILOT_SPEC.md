---
name: Japanese Reader → Anki
version: 0.1
language: Go
owner: readerer-team
license: MIT
summary: "Ingest Japanese articles/ebooks, detect unknown words/phrases, extract example sentences, and create Anki cards (AnkiConnect or .apkg)."
---

# Japanese Reader → Anki — Copilot Spec ✅

> Purpose: Automate extraction of unknown Japanese vocabulary and context from articles/ebooks and create Anki-ready flashcards for spaced repetition practice.

---

## Overview 💡

- **Scope:** Ingest text (EPUB/PDF/HTML/TXT), analyze Japanese content, surface unknown words/phrases relative to a user's known vocabulary, select example sentences, and push cards to Anki (local via AnkiConnect or `.apkg` for upload to AnkiWeb).
- **Primary user:** Japanese learners who want fast, high-quality card creation from reading material allowing for practice of new words before reading the material.

---

## Key Features (MVP → Later) 🎯

### MVP

- File/URL importer (EPUB, PDF, HTML, TXT).
- Tokenize + morphological analysis (MeCab / SudachiPy / GiNZA).
- Unknown-word detection against user-known set + common lists (JMdict, JLPT).
- Candidate review UI: accept/reject/edit cards in bulk.
- Anki integration via AnkiConnect and `.apkg` export (genanki).
- Store source metadata and tags.

### Later (Nice-to-have)

- Phrase extraction and collocation detection (n-grams + PMI).
- Automatic example sentence ranking and furigana rendering.
- Kindle support (AZW/MOBI/KFX import or Kindle export integration), respecting DRM — support only non-DRM or user-exported content.
- TTS / pronunciation audio support.
- Auto-translation suggestions (opt-in).
- Mobile/desktop reader with inline lookup & quick-add.

---

## Functional Requirements ✅

- FR1: Support `EPUB`, `PDF`, `HTML`, and `TXT` input and reliably extract Japanese text.
- FR2: Provide token attributes: surface, lemma, reading, POS, glosses (JMdict).
- FR3: Unknown detection: lemma not in user-known set (Anki decks + local list), configurable whitelist/blacklist.
- FR4: Extract and score example sentences (prefer short, single-clause examples).
- FR5: Configurable card templates and fields: `Expression`, `Reading`, `English`, `Example`, `Context`, `Source`, `Tags`.
- FR6: Push cards to Anki (AnkiConnect) or export `.apkg`.
- FR7: UI for batch operations, editing, tagging, and deck selection.
- FR8: Support Kindle formats (AZW/MOBI/KFX) for import of non-DRM or user-exported content (implementation planned for later).

---

## Non-functional Requirements ⚙️

- NFR1: Privacy-first: default to local processing; no text upload without explicit opt-in.
- NFR2: Offline support for tokenization and card generation.
- NFR3: Performance: process a 2k–5k word article in < 30s on commodity hardware.
- NFR4: Extensible plugin architecture for dictionaries and parsers.
- NFR5: Implemented in **Go** to enable fast, cross-platform single-binary distribution and easy cross-compilation.

---

## Architecture Overview 🔧

- Input layer: file/URL importer → text extractor.
- NLP layer: tokenizer + morphological analyzer → dictionary lookup (JMdict/KANJIDIC).
- Candidate selector: unknown detection, frequency filters, POS/script heuristics.
- Phrase detector: n-grams + PMI + POS heuristics.
- UI: inline reader, candidate list, card editor.
- Sync: AnkiConnect client, `.apkg` generator (genanki or Go-based exporter).
- Storage: local SQLite for vocab, history, and metadata; optional encrypted cloud sync.

Flow: Ingest → Analyze → Candidate List → Review → Card Generation → Anki Push/Export

## Implementation & Tech Stack 🛠️

- **Language:** Go (Go 1.20+), core services and CLI implemented as Go modules.
- **Build & Distribution:** `go mod` for dependency management; single static binary for distribution; GitHub Actions for cross-compilation and releases.
- **Tokenization & Morphology:** Prefer MeCab via Go bindings (e.g., `shogo82148/go-mecab`) or call external analyzers (MeCab/Sudachi) as subprocesses to preserve offline support.
- **Dictionaries:** JMdict/KANJIDIC parsed into a local SQLite database or accessed via efficient Go parsers.
- **Database:** SQLite using `modernc.org/sqlite` (CGO-free) or `mattn/go-sqlite3` (CGO) depending on distribution requirements.
- **Anki Integration:** AnkiConnect (HTTP) for push; `.apkg` export via an implemented Go exporter or by invoking `genanki` as an external tool.
- **CI & Quality:** GitHub Actions with `golangci-lint`, `go test`, and `go vet` for static analysis and testing.

---

## Data Model (Simplified) 📚

- VocabEntry: id, lemma, surface, kana, glosses[], POS[], kanjiVariants[], frequencyRank, exampleSentences[], source, tags[], createdAt, ankiDeck, cardTemplateId
- ExampleSentence: id, text, highlightedIndices, translation(optional), source, score

---

## Algorithms & Heuristics 🔍

- Unknown detection: lemma ∉ (user_known_set ∪ whitelist) AND below frequency threshold.
- Phrase extraction: compute 2–4-grams, filter by POS patterns and PMI, keep top-K.
- Example scoring: short sentences, low clause count, clear context, token prominence.

Pseudo:

```
parse doc → tokens[]
for token in tokens:
  lemma = normalize(token)
  if lemma not in known_set:
    add candidate with context
cluster candidates by lemma
select best example by scoring
```

---

## Anki Integration Details 🔁

- Preferred: AnkiConnect (POST to `localhost:8765`) for instant deck/card creation.
- Alternate: `.apkg` export using a Go-based exporter or `genanki` as an external tool for manual AnkiWeb upload.
- Recommended fields: `Expression | Reading | English | Example | Context | Source | Tags`.
- Template example: Front: `{{Expression}} <small>{{Reading}}</small>`; Back: `{{English}}` + `{{Example}}` + `{{Source}}`.

Note: AnkiWeb has no public write API; pushing to AnkiWeb requires syncing from local Anki.

---

## UI / UX ✨

- Screens: Reader view with inline lookup + quick-add, Candidate list view with bulk actions, Card editor modal, Settings.
- Keyboard-first hotkeys for power users.

---

## Privacy & Licensing ⚖️

- Respect DRM and do not bypass encrypted content.
- Use JMdict, MeCab, SudachiPy, genanki (note individual licenses). Document dependencies & licenses.
- Store user data locally by default; optional encrypted cloud sync requires explicit opt-in.

---

## Testing & Acceptance Criteria ✅

- Unit tests for tokenizer/dictionary, unknown detection, and sentence extraction; implemented as Go unit tests (`go test`).
- Static analysis and linting via `golangci-lint` and `go vet`.
- E2E: load sample EPUB → extract >95% Japanese text → candidate words appear → exported `.apkg` imports cleanly.
- Performance: 5k words < 30s on average hardware.

---

## Roadmap & Milestones 📅

1. Weeks 0–2: Project scaffold in **Go** (`go mod`), CLI and core services, text importers, MeCab integration (Go bindings or subprocess), and local SQLite DB.
2. Weeks 3–6: Unknown detection, candidate UI, AnkiConnect integration (MVP).
3. Weeks 7–10: Phrase extraction, example ranking, `.apkg` export (Go exporter or external tool).
4. Later: Kindle support, TTS, automatic translations, reader apps.

---

## Metrics 📊

- Card quality: % of auto-filled cards needing user edits < 20%.
- Throughput: avg time from reading → card creation per article.
- Retention: weekly syncs to Anki (active users).

---

## Appendix — Example Flow 🧩

1. User opens `article.epub`.
2. App extracts text and tokenizes; shows candidate unknown words.
3. User filters and accepts 12 nouns.
4. App creates cards in `Japanese:NewWords` deck via AnkiConnect; user syncs to AnkiWeb.

---

If you want edits (formatting, YAML schema for Copilot tools, or to convert to `.github/copilot.yml`), tell me which format you prefer and I will update the file. ✨
