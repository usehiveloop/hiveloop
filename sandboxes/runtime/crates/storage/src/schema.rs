/// All DDL statements for the storage layer.
///
/// Every statement uses `IF NOT EXISTS` so running migrations is idempotent.
pub const MIGRATIONS: &str = r#"
CREATE TABLE IF NOT EXISTS agents (
    agent_id    TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    version     TEXT,
    definition  BLOB NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS conversations (
    conversation_id TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    title           TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_conversations_agent
    ON conversations(agent_id);

CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id TEXT NOT NULL,
    message_index   INTEGER NOT NULL,
    role            TEXT NOT NULL,
    content         BLOB NOT NULL,
    timestamp       TEXT NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(conversation_id) ON DELETE CASCADE,
    UNIQUE(conversation_id, message_index)
);

CREATE INDEX IF NOT EXISTS idx_messages_conv
    ON messages(conversation_id, message_index);

CREATE TABLE IF NOT EXISTS webhook_outbox (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id        TEXT,
    conversation_id TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    delivered_at    TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0,
    sequence_number INTEGER
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON webhook_outbox(delivered_at)
    WHERE delivered_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_event_id
    ON webhook_outbox(event_id);

CREATE INDEX IF NOT EXISTS idx_outbox_sequence
    ON webhook_outbox(sequence_number);

CREATE TABLE IF NOT EXISTS metrics_snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT NOT NULL,
    snapshot    BLOB NOT NULL,
    captured_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_metrics_agent
    ON metrics_snapshots(agent_id, captured_at);

CREATE TABLE IF NOT EXISTS session_store (
    task_id     TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL,
    content     BLOB NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_session_agent
    ON session_store(agent_id);

-- Immortal conversations: journal entries (agent notes + checkpoints)
CREATE TABLE IF NOT EXISTS journal_entries (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    chain_index     INTEGER NOT NULL,
    entry_type      TEXT NOT NULL,
    content         BLOB NOT NULL,
    created_at      TEXT NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(conversation_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_journal_conv
    ON journal_entries(conversation_id, chain_index);

-- Immortal conversations: chain link metadata
CREATE TABLE IF NOT EXISTS chain_links (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id     TEXT NOT NULL,
    chain_index         INTEGER NOT NULL,
    started_at          TEXT NOT NULL,
    ended_at            TEXT,
    trigger_token_count INTEGER,
    checkpoint_text     BLOB,
    FOREIGN KEY (conversation_id) REFERENCES conversations(conversation_id) ON DELETE CASCADE,
    UNIQUE(conversation_id, chain_index)
);

CREATE INDEX IF NOT EXISTS idx_chain_links_conv
    ON chain_links(conversation_id);

-- Resumable artifact uploads (tus.io-style). Each row tracks an in-progress
-- or completed upload from an agent's sandbox to the control plane. The
-- idempotency key is derived from (agent_id, conversation_id, abs_path,
-- file_sha256) so a re-call of the same upload after a crash resumes from
-- the persisted offset.
CREATE TABLE IF NOT EXISTS artifact_uploads (
    idempotency_key TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    location        TEXT NOT NULL,
    total_size      INTEGER NOT NULL,
    file_sha256     TEXT NOT NULL,
    bytes_sent      INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,
    response_json   TEXT,
    last_error      TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_artifact_uploads_status
    ON artifact_uploads(status);

CREATE INDEX IF NOT EXISTS idx_artifact_uploads_agent
    ON artifact_uploads(agent_id);
"#;

/// Run all schema migrations on the given connection.
pub fn run_migrations(conn: &rusqlite::Connection) -> Result<(), rusqlite::Error> {
    conn.execute_batch(MIGRATIONS)?;
    Ok(())
}
