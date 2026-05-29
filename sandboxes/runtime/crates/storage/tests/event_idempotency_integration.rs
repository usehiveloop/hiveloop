use chrono::Utc;
use domain::{EventKind, Session, SessionId, SessionStatus};
use std::sync::atomic::{AtomicU64, Ordering};
use storage::{init_sqlite_store, EventRepo, SessionRepo, SqliteEventRepo, SqliteSessionRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

#[tokio::test]
async fn idempotent_event_append_is_enforced_by_sqlite() {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "employee-event-idempotency-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None)
        .await
        .expect("init sqlite");
    let sessions = SqliteSessionRepo::new(&store);
    let events = SqliteEventRepo::new(&store);
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
            EventKind::SpecialistEvent,
            serde_json::json!({"event_id": "event-1", "attempt": 1}),
            "specialist-callback:task-1:event-1",
        )
        .await
        .expect("first append");
    let duplicate = events
        .append_idempotent(
            &session_id,
            EventKind::SpecialistEvent,
            serde_json::json!({"event_id": "event-1", "attempt": 2}),
            "specialist-callback:task-1:event-1",
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
