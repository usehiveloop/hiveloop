CREATE INDEX IF NOT EXISTS idx_sessions_activity_id ON sessions(last_activity_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_status_activity_id ON sessions(status, last_activity_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_channel_activity_id ON sessions(channel, last_activity_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_thread_activity_id ON sessions(thread_ts, last_activity_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_session_activity_id ON sessions(agent_session_id, last_activity_at DESC, id DESC);
