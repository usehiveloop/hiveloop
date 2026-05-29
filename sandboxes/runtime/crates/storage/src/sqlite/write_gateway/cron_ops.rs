use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobSource, CronJobState};
use sqlx::SqliteConnection;

use crate::repos::Result;

const CRON_SELECT_COLS: &str = "id, description, channel, task_prompt, cron_expression, \
    interval_seconds, repeat_count, repeat_completed, state, source, next_run_at, last_run_at, \
    last_status, last_error, delegated_session_id, session_continuation_id, created_at, created_by_session, \
    agent_name, last_result";

pub(super) async fn cron_create(conn: &mut SqliteConnection, job: CronJob) -> Result<()> {
    sqlx::query(&format!(
        "INSERT INTO cron_jobs ({CRON_SELECT_COLS}) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
    ))
    .bind(job.id).bind(job.description).bind(job.channel)
    .bind(job.task_prompt).bind(job.cron_expression)
    .bind(job.interval_seconds.map(|v| v as i64))
    .bind(job.repeat_count.map(|v| v as i32)).bind(job.repeat_completed as i32)
    .bind(cron_state_str(job.state)).bind(cron_source_str(job.source))
    .bind(job.next_run_at.to_rfc3339())
    .bind(job.last_run_at.map(|t| t.to_rfc3339()))
    .bind(job.last_status).bind(job.last_error)
    .bind(job.delegated_session_id).bind(job.session_continuation_id)
    .bind(job.created_at.to_rfc3339()).bind(job.created_by_session)
    .bind(job.agent_name).bind(job.last_result)
    .execute(conn).await?;
    Ok(())
}

pub(super) async fn cron_update_prompt(
    conn: &mut SqliteConnection,
    id: &str,
    task_prompt: &str,
) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET task_prompt = ? WHERE id = ?")
        .bind(task_prompt)
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_update_interval(
    conn: &mut SqliteConnection,
    id: &str,
    interval_seconds: u64,
) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET interval_seconds = ? WHERE id = ?")
        .bind(interval_seconds as i64)
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_update_next_run(
    conn: &mut SqliteConnection,
    id: &str,
    next_run_at: DateTime<Utc>,
) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET next_run_at = ? WHERE id = ?")
        .bind(next_run_at.to_rfc3339())
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_set_state(
    conn: &mut SqliteConnection,
    id: &str,
    state: CronJobState,
) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET state = ? WHERE id = ?")
        .bind(cron_state_str(state))
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_record_run(
    conn: &mut SqliteConnection,
    id: &str,
    run_at: DateTime<Utc>,
    status: &str,
    error: Option<&str>,
) -> Result<()> {
    sqlx::query(
        "UPDATE cron_jobs SET last_run_at = ?, last_status = ?, last_error = ? WHERE id = ?",
    )
    .bind(run_at.to_rfc3339())
    .bind(status)
    .bind(error)
    .bind(id)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn cron_increment_repeat(conn: &mut SqliteConnection, id: &str) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET repeat_completed = repeat_completed + 1 WHERE id = ?")
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_record_result(
    conn: &mut SqliteConnection,
    id: &str,
    result: &str,
) -> Result<()> {
    sqlx::query("UPDATE cron_jobs SET last_status = 'completed', last_result = ? WHERE id = ?")
        .bind(result)
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn cron_complete_delegate_result(
    conn: &mut SqliteConnection,
    id: &str,
    completed_at: DateTime<Utc>,
    status: &str,
    error: Option<&str>,
    result: &str,
) -> Result<()> {
    sqlx::query(
        "UPDATE cron_jobs
         SET state = 'completed',
             last_run_at = ?,
             last_status = ?,
             last_error = ?,
             last_result = ?,
             repeat_completed = repeat_completed + 1
         WHERE id = ? AND source = 'delegate'",
    )
    .bind(completed_at.to_rfc3339())
    .bind(status)
    .bind(error)
    .bind(result)
    .bind(id)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn cron_delete(conn: &mut SqliteConnection, id: &str) -> Result<()> {
    sqlx::query("DELETE FROM cron_jobs WHERE id = ?")
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

fn cron_state_str(state: CronJobState) -> &'static str {
    match state {
        CronJobState::Active => "active",
        CronJobState::Paused => "paused",
        CronJobState::Completed => "completed",
    }
}

fn cron_source_str(source: CronJobSource) -> &'static str {
    match source {
        CronJobSource::Cron => "cron",
        CronJobSource::Delegate => "delegate",
    }
}
