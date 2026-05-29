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
        task_prompt: "test".into(),
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
        "scheduler-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

#[tokio::test]
async fn daily_report_advances_next_run_before_dispatch() {
    let repo = setup_repo().await;
    let mut job = test_job("daily", CronJobSource::Cron, 86400);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&job).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1, "overdue daily report must be due");

    let next = Utc::now() + chrono::Duration::seconds(86400);
    repo.update_next_run("daily", next).await.unwrap();

    let still_due = repo.list_due().await.unwrap();
    assert!(
        still_due.is_empty(),
        "after advancing, job must not be due again"
    );
}

#[tokio::test]
async fn wake_reminder_creates_session_continue_job() {
    let repo = setup_repo().await;
    let mut job = test_job("wake-1", CronJobSource::Cron, 300);
    job.session_continuation_id = Some("C0AS791RGLW-1778247607.836569".into());
    job.description = "wake-up reminder".into();
    repo.create(&job).await.unwrap();

    let fetched = repo.get("wake-1").await.unwrap().unwrap();
    assert_eq!(fetched.source, CronJobSource::Cron);
    assert!(
        fetched.session_continuation_id.is_some(),
        "wake reminder must preserve the session it was created in"
    );
    assert_eq!(fetched.interval_seconds, Some(300));
}

#[tokio::test]
async fn delegated_task_uses_dedicated_session_not_cron_pattern() {
    let repo = setup_repo().await;
    let mut job = test_job("delegate-1", CronJobSource::Delegate, 0);
    job.delegated_session_id = Some("C123-delegate-delegate-1".into());
    repo.create(&job).await.unwrap();

    let fetched = repo.get("delegate-1").await.unwrap().unwrap();
    assert_eq!(
        fetched.delegated_session_id.as_deref(),
        Some("C123-delegate-delegate-1")
    );
    assert_eq!(fetched.source, CronJobSource::Delegate);

    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert!(
        crons.is_empty(),
        "delegated tasks must not appear in user cron list"
    );
}

#[tokio::test]
async fn session_id_routing_by_priority() {
    let repo = setup_repo().await;

    let wake = CronJob {
        id: "wake".into(),
        session_continuation_id: Some("C123-thread-ts".into()),
        delegated_session_id: Some("C123-delegate-session".into()),
        ..test_job("wake", CronJobSource::Cron, 300)
    };
    repo.create(&wake).await.unwrap();

    let delegate = CronJob {
        id: "del".into(),
        delegated_session_id: Some("C123-delegate-session".into()),
        ..test_job("del", CronJobSource::Delegate, 0)
    };
    repo.create(&delegate).await.unwrap();

    let regular = CronJob {
        id: "reg".into(),
        ..test_job("reg", CronJobSource::Cron, 3600)
    };
    repo.create(&regular).await.unwrap();

    let wake_fetched = repo.get("wake").await.unwrap().unwrap();
    assert!(
        wake_fetched.session_continuation_id.is_some(),
        "wake: session_continuation_id must be used for thread routing"
    );

    let del_fetched = repo.get("del").await.unwrap().unwrap();
    assert!(
        del_fetched.delegated_session_id.is_some(),
        "delegate: delegated_session_id must be used for session routing"
    );

    let reg_fetched = repo.get("reg").await.unwrap().unwrap();
    assert!(
        reg_fetched.session_continuation_id.is_none() && reg_fetched.delegated_session_id.is_none(),
        "regular: no special session ID, uses auto-generated channel-cron-id"
    );
}

#[tokio::test]
async fn stale_daily_report_is_fast_forwarded_not_fired() {
    let repo = setup_repo().await;
    let mut job = test_job("stale-daily", CronJobSource::Cron, 86400);
    job.next_run_at = Utc::now() - chrono::Duration::hours(48);
    repo.create(&job).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1, "overdue job must be due");

    let is_recurring = job.interval_seconds.map(|v| v > 0).unwrap_or(false);
    assert!(is_recurring, "daily report should be treated as recurring");
    let interval = job.interval_seconds.unwrap();
    let stale_threshold = (interval as f64 * 0.5).max(120.0) as i64;
    let lag = Utc::now()
        .signed_duration_since(job.next_run_at)
        .num_seconds();
    let should_skip = lag > stale_threshold * 2;

    assert!(
        should_skip,
        "48h-stale daily report must be fast-forwarded, not fired"
    );
}

#[tokio::test]
async fn worker_cron_has_no_special_session_ids() {
    let repo = setup_repo().await;
    let mut job = test_job("worker", CronJobSource::Cron, 3600);
    job.channel = "C0AS791RGLW".into();
    job.task_prompt = "post daily summary".into();
    repo.create(&job).await.unwrap();

    let fetched = repo.get("worker").await.unwrap().unwrap();
    assert!(
        fetched.session_continuation_id.is_none(),
        "worker crons must NOT have session continuity"
    );
    assert!(
        fetched.delegated_session_id.is_none(),
        "worker crons must NOT have delegate session"
    );
    assert_eq!(fetched.channel, "C0AS791RGLW");
}
