package store

const schemaSQL = `
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_name TEXT NOT NULL,
    name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    node_type TEXT NOT NULL,
    language TEXT NOT NULL,
    body TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_path);
CREATE INDEX IF NOT EXISTS idx_symbols_lang ON symbols(language);
CREATE INDEX IF NOT EXISTS idx_symbols_repo ON symbols(repo_name);

CREATE TABLE IF NOT EXISTS repo_meta (
    repo_name TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    last_commit TEXT NOT NULL,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    symbol_count INTEGER NOT NULL,
    total_tokens INTEGER NOT NULL,
    indexed_at TEXT NOT NULL
);
`

// vecSchemaSQL is separate because it uses a format string for dimensions.
const vecSchemaSQL = `CREATE VIRTUAL TABLE IF NOT EXISTS vec_symbols USING vec0(embedding float[%d]);`
