CREATE TABLE event_idempotency_keys (
    key TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    event_id INTEGER,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_event_idempotency_session
ON event_idempotency_keys(session_id, created_at DESC);
