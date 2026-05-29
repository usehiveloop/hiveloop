use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use chrono::Utc;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use storage::{init_sqlite_store, CronJobRepo, SqliteCronJobRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

fn test_job(id: &str, source: CronJobSource, interval: u64) -> CronJob {
    CronJob {
        id: id.to_string(),
        description: "test".into(),
        channel: "C123".into(),
        task_prompt: "test prompt".into(),
        cron_expression: None,
        interval_seconds: Some(interval),
        repeat_count: None,
        repeat_completed: 0,
        state: CronJobState::Active,
        source,
        next_run_at: Utc::now(),
        last_run_at: None,
        last_status: None,
        last_error: None,
        delegated_session_id: None,
        session_continuation_id: None,
        created_at: Utc::now(),
        created_by_session: "test".into(),
        agent_name: None,
        last_result: None,
        delegate_stream_id: None,
    }
}

async fn setup_repo() -> Arc<dyn CronJobRepo> {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "delegate-wake-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

// SCENARIO: User asks agent to do a big task. Agent decides to delegate 3 background jobs in parallel.
// Each job gets a focused goal with context. Results are returned together.
#[tokio::test]
async fn delegate_parallel_creates_background_jobs_with_isolated_goals() {
    // This is what happens when the delegate tool runs in parallel mode.
    // We can't run the actual delegate tool without an LLM, so we test the cron infrastructure.
    let repo = setup_repo().await;

    // Simulate: agent delegates 3 tasks
    for (i, goal) in ["analyze issues", "check server", "audit security"]
        .iter()
        .enumerate()
    {
        let job = CronJob {
            id: format!("delegate-bg-{}", i),
            task_prompt: format!("Goal: {}", goal),
            source: CronJobSource::Delegate,
            delegated_session_id: Some(format!("delegate-{}", i)),
            ..test_job("dummy", CronJobSource::Delegate, 0)
        };
        repo.create(&job).await.unwrap();
    }

    // Verify delegated jobs are NOT in cron list
    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert!(
        crons.is_empty(),
        "delegated jobs should not appear in cron list"
    );

    // Verify delegated jobs ARE in delegate list
    let dels = repo.list_by_source(CronJobSource::Delegate).await.unwrap();
    assert_eq!(dels.len(), 3, "all 3 delegated tasks should be stored");
}

// SCENARIO: User runs a long bash command. Agent uses wake to check back in 5 minutes.
// The wake cron must have session_continuation_id to wake in the same conversation.
#[tokio::test]
async fn wake_cron_preserves_conversation_continuity() {
    let repo = setup_repo().await;
    let session_id = "C0AS791RGLW-1778247607.836569";

    // Agent creates a wake cron in the middle of a conversation
    let mut wake_job = test_job("wake-1", CronJobSource::Cron, 300);
    wake_job.session_continuation_id = Some(session_id.to_string());
    wake_job.description = "wake-up reminder".into();
    wake_job.task_prompt = "check on background build and report results".into();
    repo.create(&wake_job).await.unwrap();

    // Verify wake cron was stored with session continuity
    let fetched = repo.get("wake-1").await.unwrap().unwrap();
    assert_eq!(fetched.session_continuation_id.as_deref(), Some(session_id));
    assert_eq!(fetched.interval_seconds, Some(300));
    assert_eq!(fetched.source, CronJobSource::Cron);

    // Wake cron should be listed in cron (not delegate)
    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert_eq!(crons.len(), 1);
}

// SCENARIO: Agent schedules a recurring daily report. This is NOT a wake - it's a regular cron.
// It must NOT have session_continuation_id.
#[tokio::test]
async fn regular_cron_does_not_have_session_continuity() {
    let repo = setup_repo().await;
    let daily = test_job("daily-report", CronJobSource::Cron, 86400);
    repo.create(&daily).await.unwrap();

    let fetched = repo.get("daily-report").await.unwrap().unwrap();
    assert!(
        fetched.session_continuation_id.is_none(),
        "regular recurring crons should NOT have session continuity"
    );
    assert_eq!(fetched.interval_seconds, Some(86400));
}

// SCENARIO: User asks agent to list their cron jobs. Only user-created crons appear.
// Delegated background tasks are invisible.
#[tokio::test]
async fn user_only_sees_their_cron_jobs_not_delegated() {
    let repo = setup_repo().await;

    // User creates 2 cron jobs
    repo.create(&test_job("morning-report", CronJobSource::Cron, 86400))
        .await
        .unwrap();
    repo.create(&test_job("afternoon-check", CronJobSource::Cron, 43200))
        .await
        .unwrap();

    // Agent creates 1 delegated background task
    let mut bg = test_job("bg-task", CronJobSource::Delegate, 0);
    bg.delegated_session_id = Some("del-session".into());
    repo.create(&bg).await.unwrap();

    // User lists crons - should only see their 2
    let user_crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert_eq!(user_crons.len(), 2, "user should only see their cron jobs");
}

// SCENARIO: Agent tries to cancel a delegated background task using cron tool.
// This must be rejected - only check_delegated_status can touch delegate jobs.
#[tokio::test]
async fn cron_tool_cannot_manage_delegated_jobs() {
    let repo = setup_repo().await;

    let mut bg = test_job("bg-1", CronJobSource::Delegate, 0);
    bg.delegated_session_id = Some("session-1".into());
    repo.create(&bg).await.unwrap();

    // Verify it exists in delegate list
    let dels = repo.list_by_source(CronJobSource::Delegate).await.unwrap();
    assert_eq!(dels.len(), 1);

    // Verify it does NOT exist in cron list (the guard is list_by_source)
    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert!(!crons.iter().any(|c| c.id == "bg-1"));
}

// SCENARIO: Multiple simultaneous wake crons for different conversations.
// Each preserves its own session_continuation_id independently.
#[tokio::test]
async fn multiple_wake_crons_preserve_different_sessions() {
    let repo = setup_repo().await;

    for (session, interval) in [
        ("C123-thread-1", 300),
        ("C456-thread-2", 600),
        ("C789-thread-3", 120),
    ] {
        let mut job = test_job(&format!("wake-{}", session), CronJobSource::Cron, interval);
        job.session_continuation_id = Some(session.to_string());
        repo.create(&job).await.unwrap();
    }

    let all = repo.list_all().await.unwrap();
    assert_eq!(all.len(), 3);

    let sessions: Vec<_> = all
        .iter()
        .filter_map(|j| j.session_continuation_id.as_deref())
        .collect();
    assert!(sessions.contains(&"C123-thread-1"));
    assert!(sessions.contains(&"C456-thread-2"));
    assert!(sessions.contains(&"C789-thread-3"));
}
