use chrono::Utc;
use domain::{EventKind, Session, SessionId, SessionStatus};
use storage::{init_sqlite_pool, EventRepo, SessionRepo, SqliteEventRepo, SqliteSessionRepo};

#[tokio::test]
async fn idempotent_event_append_is_enforced_by_sqlite() {
    let db_path = std::env::temp_dir().join(format!(
        "employee-event-idempotency-{}.db",
        Utc::now().timestamp_nanos_opt().unwrap_or_default()
    ));
    let pool = init_sqlite_pool(&db_path).await.expect("init sqlite");
    let sessions = SqliteSessionRepo::new(pool.clone());
    let events = SqliteEventRepo::new(pool);
    let now = Utc::now();
    let session_id = SessionId::from("session-1");
    sessions
        .create(&Session {
            id: session_id.clone(),
            channel: "http".to_string(),
            thread_ts: "thread-1".to_string(),
            agent_session_id: "session-1".to_string(),
            status: SessionStatus::Active,
            created_at: now,
            last_activity_at: now,
        })
        .await
        .expect("create session");

    let first = events
        .append_idempotent(
            &session_id,
            EventKind::CloudAgentEvent,
            serde_json::json!({"event_id": "event-1", "attempt": 1}),
            "cloud-agent-callback:task-1:event-1",
        )
        .await
        .expect("first append");
    let duplicate = events
        .append_idempotent(
            &session_id,
            EventKind::CloudAgentEvent,
            serde_json::json!({"event_id": "event-1", "attempt": 2}),
            "cloud-agent-callback:task-1:event-1",
        )
        .await
        .expect("duplicate append");

    assert!(first.is_some());
    assert_eq!(duplicate, None);
    let stored = events
        .list_recent(&session_id, 10)
        .await
        .expect("list events");
    assert_eq!(stored.len(), 1);
    assert_eq!(stored[0].payload["attempt"], 1);

    let _ = std::fs::remove_file(db_path);
}
