-- migrations for readerer DB

CREATE TABLE IF NOT EXISTS words (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word TEXT NOT NULL,
    lemma TEXT,
    language TEXT DEFAULT 'und',
    pronunciation TEXT,
    image_url TEXT,
    mnemonic_text TEXT,
    definitions TEXT,
    UNIQUE(word, lemma, language)
);

CREATE TABLE IF NOT EXISTS sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type TEXT NOT NULL,
    title TEXT,
    author TEXT,
    website TEXT,
    url TEXT,
    meta TEXT,
    last_processed_sentence INTEGER DEFAULT -1,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sources_unique ON sources(url, title, author);

CREATE TABLE IF NOT EXISTS word_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    context_sentence TEXT,
    example_sentence TEXT,
    occurrence_count INTEGER DEFAULT 1,
    first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_primary INTEGER DEFAULT 0,
    UNIQUE(word_id, source_id)
);

CREATE INDEX IF NOT EXISTS idx_word_sources_source_id ON word_sources(source_id);
CREATE INDEX IF NOT EXISTS idx_word_sources_word_id ON word_sources(word_id);

CREATE TABLE IF NOT EXISTS word_contexts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_source_id INTEGER NOT NULL REFERENCES word_sources(id) ON DELETE CASCADE,
    sentence TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(word_source_id, sentence)
);

CREATE INDEX IF NOT EXISTS idx_word_contexts_ws_id ON word_contexts(word_source_id);

-- New sentences table to deduplicate repeated sentences across tables
CREATE TABLE IF NOT EXISTS sentences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    text TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed the `sentences` table from existing sentence-like columns so that
-- subsequent conversion queries can resolve sentence IDs when upgrading an
-- existing database. On a fresh database, these inserts are effectively no-ops.
INSERT OR IGNORE INTO sentences(text)
    SELECT DISTINCT context_sentence
    FROM word_sources
    WHERE context_sentence IS NOT NULL;

INSERT OR IGNORE INTO sentences(text)
    SELECT DISTINCT example_sentence
    FROM word_sources
    WHERE example_sentence IS NOT NULL;

INSERT OR IGNORE INTO sentences(text)
    SELECT DISTINCT sentence
    FROM word_contexts
    WHERE sentence IS NOT NULL;

-- Create new tables that reference sentences by id
CREATE TABLE IF NOT EXISTS new_word_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    context_sentence_id INTEGER REFERENCES sentences(id) ON DELETE SET NULL,
    example_sentence_id INTEGER REFERENCES sentences(id) ON DELETE SET NULL,
    occurrence_count INTEGER DEFAULT 1,
    first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_primary INTEGER DEFAULT 0,
    UNIQUE(word_id, source_id)
);

-- Copy existing word_sources rows, resolving sentence ids
INSERT OR IGNORE INTO new_word_sources (id, word_id, source_id, context_sentence_id, example_sentence_id, occurrence_count, first_seen_at, is_primary)
SELECT ws.id, ws.word_id, ws.source_id,
  (SELECT id FROM sentences s WHERE s.text = ws.context_sentence),
  (SELECT id FROM sentences s WHERE s.text = ws.example_sentence),
  ws.occurrence_count, ws.first_seen_at, ws.is_primary
FROM word_sources ws;

CREATE TABLE IF NOT EXISTS new_word_contexts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_source_id INTEGER NOT NULL REFERENCES new_word_sources(id) ON DELETE CASCADE,
    sentence_id INTEGER NOT NULL REFERENCES sentences(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(word_source_id, sentence_id)
);

-- Copy existing contexts resolving sentence ids
INSERT OR IGNORE INTO new_word_contexts (id, word_source_id, sentence_id, created_at)
SELECT wc.id, wc.word_source_id, (SELECT id FROM sentences s WHERE s.text = wc.sentence), wc.created_at
FROM word_contexts wc;

-- Replace old tables with new ones
DROP TABLE IF EXISTS word_contexts;
DROP TABLE IF EXISTS word_sources;
ALTER TABLE new_word_sources RENAME TO word_sources;
ALTER TABLE new_word_contexts RENAME TO word_contexts;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_word_sources_source_id ON word_sources(source_id);
CREATE INDEX IF NOT EXISTS idx_word_sources_word_id ON word_sources(word_id);
CREATE INDEX IF NOT EXISTS idx_word_contexts_ws_id ON word_contexts(word_source_id);

