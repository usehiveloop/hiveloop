CREATE TABLE agent_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    definition_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    channel TEXT NOT NULL,
    thread_ts TEXT NOT NULL,
    adk_session_id TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_activity_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_activity ON sessions(last_activity_at DESC);
CREATE INDEX idx_sessions_status ON sessions(status);

CREATE TABLE session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_session_events_session_seq ON session_events(session_id, seq);
CREATE INDEX idx_session_events_session_created ON session_events(session_id, created_at DESC);

CREATE TABLE inbound_dedupe (
    envelope_id TEXT PRIMARY KEY,
    received_at TEXT NOT NULL
);
CREATE INDEX idx_dedupe_received ON inbound_dedupe(received_at);

CREATE TABLE events_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    recorded_at TEXT NOT NULL
);
CREATE INDEX idx_events_log_type ON events_log(event_type, recorded_at DESC);
CREATE INDEX idx_events_log_recorded ON events_log(recorded_at DESC);

CREATE TABLE outbound_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_outbox_due ON outbound_outbox(status, next_retry_at);
CREATE INDEX idx_outbox_channel ON outbound_outbox(channel_name);
