CREATE VIRTUAL TABLE IF NOT EXISTS session_event_search USING fts5(
    event_id UNINDEXED,
    session_id UNINDEXED,
    kind UNINDEXED,
    content,
    created_at UNINDEXED
);

INSERT INTO session_event_search (event_id, session_id, kind, content, created_at)
SELECT
    CAST(id AS TEXT),
    session_id,
    kind,
    CASE
        WHEN kind IN ('user_message', 'assistant_message', 'tool_result') THEN COALESCE(json_extract(payload_json, '$.message.parts[0].text'), '')
        WHEN kind = 'tool_call' THEN COALESCE(json_extract(payload_json, '$.message.tool_calls'), '')
        ELSE ''
    END,
    created_at
FROM session_events
WHERE kind IN ('user_message', 'assistant_message', 'tool_call', 'tool_result')
  AND TRIM(CASE
        WHEN kind IN ('user_message', 'assistant_message', 'tool_result') THEN COALESCE(json_extract(payload_json, '$.message.parts[0].text'), '')
        WHEN kind = 'tool_call' THEN COALESCE(json_extract(payload_json, '$.message.tool_calls'), '')
        ELSE ''
    END) <> ''
  AND NOT EXISTS (
      SELECT 1 FROM session_event_search WHERE session_event_search.event_id = CAST(session_events.id AS TEXT)
  );
