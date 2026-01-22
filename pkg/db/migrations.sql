-- migrations for readerer DB

CREATE TABLE IF NOT EXISTS words (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word TEXT NOT NULL,
    lemma TEXT,
    language TEXT DEFAULT 'und',
    pronunciation TEXT,
    image_url TEXT,
    mnemonic_text TEXT,
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
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

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
