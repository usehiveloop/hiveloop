use chrono::Utc;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use storage::{init_sqlite_store, CronJobRepo, SqliteCronJobRepo};

static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

fn test_job(id: &str, interval: u64) -> CronJob {
    CronJob {
        id: id.to_string(),
        description: "test job".into(),
        channel: "C123".into(),
        task_prompt: "test prompt".into(),
        cron_expression: None,
        interval_seconds: Some(interval),
        repeat_count: None,
        repeat_completed: 0,
        state: CronJobState::Active,
        source: CronJobSource::Cron,
        next_run_at: Utc::now(),
        last_run_at: None,
        last_status: None,
        last_error: None,
        delegated_session_id: None,
        session_continuation_id: None,
        created_at: Utc::now(),
        created_by_session: "test-session".into(),
        agent_name: None,
        last_result: None,
    }
}

async fn setup_repo() -> Arc<dyn CronJobRepo> {
    let unique = DB_COUNTER.fetch_add(1, Ordering::Relaxed);
    let db_path = std::env::temp_dir().join(format!(
        "cron-integration-{}-{unique}.db",
        std::process::id()
    ));
    let store = init_sqlite_store(&db_path, None).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(&store))
}

#[tokio::test]
async fn create_and_get_job() {
    let repo = setup_repo().await;
    let job = test_job("job-1", 60);
    repo.create(&job).await.unwrap();
    let fetched = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(fetched.id, "job-1");
    assert_eq!(fetched.task_prompt, "test prompt");
    assert_eq!(fetched.state, CronJobState::Active);
    assert_eq!(fetched.interval_seconds, Some(60));
}

#[tokio::test]
async fn list_all_returns_created_jobs() {
    let repo = setup_repo().await;
    repo.create(&test_job("a", 60)).await.unwrap();
    repo.create(&test_job("b", 120)).await.unwrap();
    let all = repo.list_all().await.unwrap();
    assert_eq!(all.len(), 2);
}

#[tokio::test]
async fn list_due_only_returns_active_and_due() {
    let repo = setup_repo().await;
    let mut job = test_job("active-due", 60);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&job).await.unwrap();

    let mut past = test_job("paused-due", 60);
    past.state = CronJobState::Paused;
    past.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&past).await.unwrap();

    let mut future = test_job("active-future", 60);
    future.next_run_at = Utc::now() + chrono::Duration::seconds(3600);
    repo.create(&future).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);
    assert_eq!(due[0].id, "active-due");
}

#[tokio::test]
async fn list_due_does_not_redispatch_running_delegate_with_active_lease() {
    let repo = setup_repo().await;
    let mut job = test_job("delegate-running", 0);
    job.source = CronJobSource::Delegate;
    job.delegated_session_id = Some("parent-delegate-delegate-running".into());
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    job.last_run_at = Some(Utc::now() - chrono::Duration::seconds(60));
    job.last_status = Some("running".into());
    repo.create(&job).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert!(
        due.is_empty(),
        "running delegate lease must prevent redispatch"
    );
}

#[tokio::test]
async fn list_due_allows_stale_running_delegate_recovery() {
    let repo = setup_repo().await;
    let mut job = test_job("delegate-stale", 0);
    job.source = CronJobSource::Delegate;
    job.delegated_session_id = Some("parent-delegate-delegate-stale".into());
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    job.last_run_at = Some(Utc::now() - chrono::Duration::minutes(31));
    job.last_status = Some("running".into());
    repo.create(&job).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);
    assert_eq!(due[0].id, "delegate-stale");
}

#[tokio::test]
async fn complete_delegate_result_marks_job_terminal() {
    let repo = setup_repo().await;
    let mut job = test_job("delegate-done", 0);
    job.source = CronJobSource::Delegate;
    job.repeat_count = Some(1);
    job.delegated_session_id = Some("parent-delegate-delegate-done".into());
    repo.create(&job).await.unwrap();

    repo.complete_delegate_result(
        "delegate-done",
        Utc::now(),
        "failed",
        Some("HTTP 402"),
        "[Sub-agent error: HTTP 402]",
    )
    .await
    .unwrap();

    let fetched = repo.get("delegate-done").await.unwrap().unwrap();
    assert_eq!(fetched.state, CronJobState::Completed);
    assert_eq!(fetched.last_status.as_deref(), Some("failed"));
    assert_eq!(fetched.last_error.as_deref(), Some("HTTP 402"));
    assert_eq!(
        fetched.last_result.as_deref(),
        Some("[Sub-agent error: HTTP 402]")
    );
    assert_eq!(fetched.repeat_completed, 1);
    assert!(repo.list_due().await.unwrap().is_empty());
}

#[tokio::test]
async fn set_state_transitions() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    repo.set_state("job-1", CronJobState::Paused).await.unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.state, CronJobState::Paused);
    repo.set_state("job-1", CronJobState::Active).await.unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.state, CronJobState::Active);
}

#[tokio::test]
async fn record_run_updates_last_run_and_status() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    let now = Utc::now();
    repo.record_run("job-1", now, "ok", None).await.unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert!(job.last_run_at.is_some());
    assert_eq!(job.last_status.as_deref(), Some("ok"));
    assert!(job.last_error.is_none());

    repo.record_run("job-1", now, "error", Some("timeout"))
        .await
        .unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.last_status.as_deref(), Some("error"));
    assert_eq!(job.last_error.as_deref(), Some("timeout"));
}

#[tokio::test]
async fn repeat_count_tracks_completions() {
    let repo = setup_repo().await;
    let mut job = test_job("repeat-me", 60);
    job.repeat_count = Some(3);
    repo.create(&job).await.unwrap();

    repo.increment_repeat("repeat-me").await.unwrap();
    let job = repo.get("repeat-me").await.unwrap().unwrap();
    assert_eq!(job.repeat_completed, 1);

    repo.increment_repeat("repeat-me").await.unwrap();
    repo.increment_repeat("repeat-me").await.unwrap();
    let job = repo.get("repeat-me").await.unwrap().unwrap();
    assert_eq!(job.repeat_completed, 3);
}

#[tokio::test]
async fn delete_removes_job() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    repo.delete("job-1").await.unwrap();
    assert!(repo.get("job-1").await.unwrap().is_none());
}

#[tokio::test]
async fn update_prompt_changes_task_prompt() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    repo.update_prompt("job-1", "new prompt".into())
        .await
        .unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.task_prompt, "new prompt");
}

#[tokio::test]
async fn update_interval_changes_interval() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    repo.update_interval("job-1", 300).await.unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    assert_eq!(job.interval_seconds, Some(300));
}

#[tokio::test]
async fn update_next_run_advances_schedule() {
    let repo = setup_repo().await;
    repo.create(&test_job("job-1", 60)).await.unwrap();
    let future = Utc::now() + chrono::Duration::seconds(3600);
    repo.update_next_run("job-1", future).await.unwrap();
    let job = repo.get("job-1").await.unwrap().unwrap();
    let diff = (job.next_run_at - future).num_seconds().abs();
    assert!(diff < 2);
}

#[tokio::test]
async fn at_most_once_next_run_advances_before_dispatch() {
    let repo = setup_repo().await;
    let mut job = test_job("at-most-once", 60);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&job).await.unwrap();

    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);

    let next = Utc::now() + chrono::Duration::seconds(60);
    repo.update_next_run("at-most-once", next).await.unwrap();

    let due_after = repo.list_due().await.unwrap();
    assert!(due_after.is_empty());
}

#[tokio::test]
async fn paused_jobs_are_not_due() {
    let repo = setup_repo().await;
    let mut job = test_job("paused-not-due", 60);
    job.state = CronJobState::Paused;
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.create(&job).await.unwrap();
    let due = repo.list_due().await.unwrap();
    assert!(due.is_empty());
}

#[tokio::test]
async fn completed_state_persists() {
    let repo = setup_repo().await;
    repo.create(&test_job("done", 60)).await.unwrap();
    repo.set_state("done", CronJobState::Completed)
        .await
        .unwrap();
    let job = repo.get("done").await.unwrap().unwrap();
    assert_eq!(job.state, CronJobState::Completed);
}

#[tokio::test]
async fn wake_cron_stores_session_continuation_id() {
    let repo = setup_repo().await;
    let mut job = test_job("wake-1", 300);
    job.session_continuation_id = Some("C123-1778211804".into());
    repo.create(&job).await.unwrap();
    let fetched = repo.get("wake-1").await.unwrap().unwrap();
    assert_eq!(
        fetched.session_continuation_id.as_deref(),
        Some("C123-1778211804")
    );
    assert_eq!(fetched.interval_seconds, Some(300));
}

#[tokio::test]
async fn wake_cron_next_run_is_now_plus_interval() {
    let repo = setup_repo().await;
    let now = Utc::now();
    let mut job = test_job("wake-2", 300);
    job.next_run_at = now + chrono::Duration::seconds(300);
    job.session_continuation_id = Some("C123-sess".into());
    repo.create(&job).await.unwrap();
    let fetched = repo.get("wake-2").await.unwrap().unwrap();
    let diff = (fetched.next_run_at - (now + chrono::Duration::seconds(300)))
        .num_seconds()
        .abs();
    assert!(
        diff < 2,
        "next_run_at should be ~now+300s, got diff={}",
        diff
    );
    assert!(fetched.session_continuation_id.is_some());
}

#[tokio::test]
async fn list_by_source_excludes_delegated() {
    let repo = setup_repo().await;
    let mut regular = test_job("regular", 60);
    regular.source = CronJobSource::Cron;
    repo.create(&regular).await.unwrap();

    let mut delegated = test_job("delegated", 60);
    delegated.source = CronJobSource::Delegate;
    repo.create(&delegated).await.unwrap();

    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert_eq!(crons.len(), 1);
    assert_eq!(crons[0].id, "regular");

    let dels = repo.list_by_source(CronJobSource::Delegate).await.unwrap();
    assert_eq!(dels.len(), 1);
    assert_eq!(dels[0].id, "delegated");
}

#[tokio::test]
async fn multiple_wake_crons_independent() {
    let repo = setup_repo().await;
    for i in 1..=3 {
        let mut job = test_job(&format!("wake-{}", i), 120);
        job.session_continuation_id = Some(format!("session-{}", i));
        repo.create(&job).await.unwrap();
    }
    let all = repo.list_all().await.unwrap();
    assert_eq!(all.len(), 3);
    let ids: Vec<_> = all
        .iter()
        .map(|j| j.session_continuation_id.as_deref().unwrap())
        .collect();
    assert!(ids.contains(&"session-1"));
    assert!(ids.contains(&"session-2"));
    assert!(ids.contains(&"session-3"));
}

#[tokio::test]
async fn wake_cron_is_due_when_next_run_passed() {
    let repo = setup_repo().await;
    let mut job = test_job("wake-due", 60);
    job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    job.session_continuation_id = Some("C123-cont".into());
    repo.create(&job).await.unwrap();
    let due = repo.list_due().await.unwrap();
    assert_eq!(due.len(), 1);
    assert_eq!(due[0].id, "wake-due");
    assert_eq!(due[0].session_continuation_id.as_deref(), Some("C123-cont"));
}
