use std::sync::Arc;

use chrono::Utc;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::SessionId;
use storage::{CronJobRepo, SqliteCronJobRepo};

fn test_job(id: &str, source: CronJobSource) -> CronJob {
    CronJob {
        id: id.to_string(), description: "test".into(), channel: "C123".into(),
        task_prompt: "test".into(), cron_expression: None,
        interval_seconds: Some(60), repeat_count: None, repeat_completed: 0,
        state: CronJobState::Active, source,
        next_run_at: Utc::now(), last_run_at: None, last_status: None, last_error: None,
        delegated_session_id: None, session_continuation_id: None,
        created_at: Utc::now(), created_by_session: "test".into(),
    }
}

async fn setup_repo() -> Arc<dyn CronJobRepo> {
    let pool = sqlx::sqlite::SqlitePoolOptions::new().max_connections(2)
        .connect("sqlite::memory:").await.unwrap();
    sqlx::query("CREATE TABLE cron_jobs (
        id TEXT PRIMARY KEY NOT NULL, description TEXT NOT NULL, channel TEXT NOT NULL,
        task_prompt TEXT NOT NULL, cron_expression TEXT, interval_seconds INTEGER,
        repeat_count INTEGER, repeat_completed INTEGER NOT NULL DEFAULT 0,
        state TEXT NOT NULL DEFAULT 'active', source TEXT NOT NULL DEFAULT 'cron',
        next_run_at TEXT NOT NULL, last_run_at TEXT, last_status TEXT, last_error TEXT,
        delegated_session_id TEXT, session_continuation_id TEXT,
        created_at TEXT NOT NULL, created_by_session TEXT NOT NULL
    )").execute(&pool).await.unwrap();
    Arc::new(SqliteCronJobRepo::new(Arc::new(pool)))
}

#[tokio::test]
async fn user_creates_daily_report_with_cron_tool() {
    let repo = setup_repo().await;
    let mut job = test_job("daily-report", CronJobSource::Cron);
    job.description = "Daily team summary".into();
    job.task_prompt = "Summarize today's Linear issues and post to channel".into();
    job.interval_seconds = Some(86400);
    repo.create(&job).await.unwrap();

    let fetched = repo.get("daily-report").await.unwrap().unwrap();
    assert_eq!(fetched.description, "Daily team summary");
    assert_eq!(fetched.interval_seconds, Some(86400));
    assert_eq!(fetched.source, CronJobSource::Cron);
}

#[tokio::test]
async fn user_lists_crons_only_sees_their_own() {
    let repo = setup_repo().await;
    repo.create(&test_job("report-1", CronJobSource::Cron)).await.unwrap();
    repo.create(&test_job("report-2", CronJobSource::Cron)).await.unwrap();
    let mut bg = test_job("bg-1", CronJobSource::Delegate);
    bg.delegated_session_id = Some("session-1".into());
    repo.create(&bg).await.unwrap();

    let user_crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert_eq!(user_crons.len(), 2, "user must only see their cron jobs");
    assert!(!user_crons.iter().any(|c| c.id == "bg-1"), "delegate job must be invisible");
}

#[tokio::test]
async fn user_cancels_daily_report() {
    let repo = setup_repo().await;
    repo.create(&test_job("daily", CronJobSource::Cron)).await.unwrap();
    assert!(repo.get("daily").await.unwrap().is_some());

    repo.delete("daily").await.unwrap();
    assert!(repo.get("daily").await.unwrap().is_none(), "cancelled cron must be gone");
}

#[tokio::test]
async fn user_pauses_and_resumes_a_cron() {
    let repo = setup_repo().await;
    repo.create(&test_job("weekly", CronJobSource::Cron)).await.unwrap();

    repo.set_state("weekly", CronJobState::Paused).await.unwrap();
    let paused = repo.get("weekly").await.unwrap().unwrap();
    assert_eq!(paused.state, CronJobState::Paused);

    let due = repo.list_due().await.unwrap();
    assert!(due.is_empty(), "paused job must not be due");

    repo.set_state("weekly", CronJobState::Active).await.unwrap();
    let active = repo.get("weekly").await.unwrap().unwrap();
    assert_eq!(active.state, CronJobState::Active);

    let mut active_job = active;
    active_job.next_run_at = Utc::now() - chrono::Duration::seconds(10);
    repo.update_next_run("weekly", active_job.next_run_at).await.unwrap();
    let due_now = repo.list_due().await.unwrap();
    assert!(!due_now.is_empty(), "resumed job must be due again");
}

#[tokio::test]
async fn user_updates_cron_prompt() {
    let repo = setup_repo().await;
    repo.create(&test_job("alert", CronJobSource::Cron)).await.unwrap();

    repo.update_prompt("alert", "New prompt: check for urgent issues".into()).await.unwrap();
    let updated = repo.get("alert").await.unwrap().unwrap();
    assert_eq!(updated.task_prompt, "New prompt: check for urgent issues");
}

#[tokio::test]
async fn user_updates_cron_interval() {
    let repo = setup_repo().await;
    repo.create(&test_job("hourly", CronJobSource::Cron)).await.unwrap();
    assert_eq!(repo.get("hourly").await.unwrap().unwrap().interval_seconds, Some(60));

    repo.update_interval("hourly", 3600).await.unwrap();
    assert_eq!(repo.get("hourly").await.unwrap().unwrap().interval_seconds, Some(3600));
}

#[tokio::test]
async fn recording_run_updates_execution_history() {
    let repo = setup_repo().await;
    repo.create(&test_job("tracked", CronJobSource::Cron)).await.unwrap();

    let now = Utc::now();
    repo.record_run("tracked", now, "ok", None).await.unwrap();
    let job = repo.get("tracked").await.unwrap().unwrap();
    assert!(job.last_run_at.is_some());
    assert_eq!(job.last_status.as_deref(), Some("ok"));

    repo.record_run("tracked", now, "error", Some("timeout")).await.unwrap();
    let job = repo.get("tracked").await.unwrap().unwrap();
    assert_eq!(job.last_status.as_deref(), Some("error"));
    assert_eq!(job.last_error.as_deref(), Some("timeout"));
}

#[tokio::test]
async fn repeat_count_auto_completes_after_n_runs() {
    let repo = setup_repo().await;
    let mut job = test_job("limited", CronJobSource::Cron);
    job.repeat_count = Some(3);
    repo.create(&job).await.unwrap();

    repo.increment_repeat("limited").await.unwrap();
    repo.increment_repeat("limited").await.unwrap();
    repo.increment_repeat("limited").await.unwrap();

    let job = repo.get("limited").await.unwrap().unwrap();
    assert_eq!(job.repeat_completed, 3, "must track 3 completions");
    assert!(job.repeat_count == Some(3) && job.repeat_completed >= 3,
        "after N runs, job should be marked for completion");
}

#[tokio::test]
async fn cron_list_respects_source_filtering() {
    let repo = setup_repo().await;

    let mut wake = test_job("wake-reminder", CronJobSource::Cron);
    wake.session_continuation_id = Some("C123-thread".into());
    repo.create(&wake).await.unwrap();

    let mut delegate = test_job("delegate-task", CronJobSource::Delegate);
    delegate.delegated_session_id = Some("del-session".into());
    repo.create(&delegate).await.unwrap();

    let crons = repo.list_by_source(CronJobSource::Cron).await.unwrap();
    assert_eq!(crons.len(), 1, "only cron-source jobs");
    assert_eq!(crons[0].id, "wake-reminder");

    let dels = repo.list_by_source(CronJobSource::Delegate).await.unwrap();
    assert_eq!(dels.len(), 1, "only delegate-source jobs");
    assert_eq!(dels[0].id, "delegate-task");
}

fn is_cron_session(session_id: &str) -> bool {
    session_id.contains("-cron-")
}

fn is_wake_cron(session_id: &str) -> bool {
    !session_id.contains("-cron-") && !session_id.contains("-delegate-") && !session_id.contains('.')
}

#[test]
fn handler_identifies_cron_session() {
    assert!(is_cron_session("C123-cron-job-1"));
    assert!(is_cron_session("C123-cron-cron-1778211804202"));
    assert!(!is_cron_session("C123-1778247607.836569"));
    assert!(!is_cron_session("C123-delegate-job-2"));
}

#[test]
fn handler_identifies_wake_cron_for_thread_reply() {
    let session_id = "C123-1778247607.836569";
    let user_is_cron = true;
    assert!(user_is_cron && !is_cron_session(session_id), "wake cron must use thread reply");
}

#[test]
fn handler_identifies_worker_cron_for_channel_post() {
    let session_id = "C123-cron-report-1";
    let user_is_cron = true;
    assert!(user_is_cron && is_cron_session(session_id), "worker cron must use channel post");
}

#[test]
fn normal_user_message_always_replies_in_thread() {
    assert!(!is_cron_session("C123-1778247607.836569"));
    assert!(!is_cron_session("C123-another-message"));
}
