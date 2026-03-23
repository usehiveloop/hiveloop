use crate::error::StorageError;

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
    conversation_id TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         BLOB NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    delivered_at    TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON webhook_outbox(delivered_at)
    WHERE delivered_at IS NULL;

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
"#;

/// Run all schema migrations on the given connection.
pub async fn run_migrations(conn: &libsql::Connection) -> Result<(), StorageError> {
    conn.execute_batch(MIGRATIONS)
        .await
        .map_err(|e| StorageError::Database(format!("migration failed: {e}")))?;
    Ok(())
}
