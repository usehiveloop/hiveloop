use domain::{EventKind, Session, SessionId, SessionStatus};
use storage::{init_sqlite_store, EventRepo, SessionRepo, SqliteEventRepo, SqliteSessionRepo};

fn history_payload(text: &str, role: &str) -> serde_json::Value {
    serde_json::json!({
        "version": 1,
        "message": {
            "role": role,
            "parts": [{"type": "text", "text": text}],
            "tool_calls": [],
            "tool_call_id": null
        }
    })
}

#[tokio::test]
async fn search_sessions_finds_readable_conversation_rows() {
    let dir = tempfile::tempdir().expect("tempdir");
    let db_path = dir.path().join("runtime.db");
    let store = init_sqlite_store(&db_path, None)
        .await
        .expect("init sqlite");
    let sessions = SqliteSessionRepo::new(&store);
    let events = SqliteEventRepo::new(&store);
    let now = chrono::Utc::now();
    let session_id = SessionId::from("session-search-1");
    sessions
        .create(&Session {
            id: session_id.clone(),
            channel: "http".into(),
            thread_ts: "thread".into(),
            agent_session_id: "agent-session".into(),
            status: SessionStatus::Active,
            created_at: now,
            last_activity_at: now,
        })
        .await
        .expect("create session");

    events
        .append(
            &session_id,
            EventKind::UserMessage,
            history_payload("Please remember the Acme launch checklist", "user"),
        )
        .await
        .expect("append user");
    events
        .append(
            &session_id,
            EventKind::RunEvent,
            serde_json::json!({"event":"model_usage","secret":"not searchable"}),
        )
        .await
        .expect("append run");

    let matches = events
        .search_sessions("launch checklist", None, 10)
        .await
        .expect("search");
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].session_id, "session-search-1");
    assert!(matches[0].snippet.contains("launch"));
}
