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

-- Legacy, text-based `word_sources` columns removed; keep migration SQL focused
-- on final, id-based schema. Upgrades from older DBs are not handled here.

CREATE TABLE IF NOT EXISTS sentences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    text TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS word_sources (
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

CREATE INDEX IF NOT EXISTS idx_word_sources_source_id ON word_sources(source_id);
CREATE INDEX IF NOT EXISTS idx_word_sources_word_id ON word_sources(word_id);

CREATE TABLE IF NOT EXISTS word_contexts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_source_id INTEGER NOT NULL REFERENCES word_sources(id) ON DELETE CASCADE,
    sentence_id INTEGER NOT NULL REFERENCES sentences(id) ON DELETE CASCADE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(word_source_id, sentence_id)
);

CREATE INDEX IF NOT EXISTS idx_word_contexts_ws_id ON word_contexts(word_source_id);

