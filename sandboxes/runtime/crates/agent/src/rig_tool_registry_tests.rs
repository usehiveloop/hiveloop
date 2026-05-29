use super::*;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::ToolSpec;
use outbound::{OutboundChannel, OutboundError, OutboundRegistry};
use std::collections::HashMap;
use std::fs;
use std::sync::Mutex;
use storage::{OutboxRepo, OutboxRow};
use tokio::sync::RwLock;

fn test_agent_definition() -> domain::AgentDefinition {
    domain::AgentDefinition {
        agent: domain::AgentMeta {
            name: "Test".to_string(),
            description: "Test agent".to_string(),
        },
        mode: Default::default(),
        specialist_profile: None,
        system_prompt: Default::default(),
        model: domain::ModelConfig::OpenaiCompatible {
            base_url: "http://localhost".to_string(),
            model_id: "test".to_string(),
            api_key_env: "TEST_KEY".to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: Default::default(),
            fallback: None,
        },
        multimodal_model: None,
        limits: Default::default(),
        context: Default::default(),
        tools: Vec::new(),
        mcp_servers: Vec::new(),
        skills: Vec::new(),
        outbound_channels: Vec::new(),
        sub_agents: Default::default(),
    }
}

#[derive(Default)]
struct FakeOutbox {
    rows: Mutex<Vec<(String, String, Value)>>,
}

#[derive(Default)]
struct FakeCronRepo {
    jobs: Mutex<HashMap<String, CronJob>>,
}

#[async_trait]
impl OutboxRepo for FakeOutbox {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: Value,
    ) -> storage::Result<i64> {
        let mut rows = self.rows.lock().expect("outbox lock");
        rows.push((channel_name.to_string(), event_type.to_string(), payload));
        Ok(rows.len() as i64)
    }

    async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
        Ok(Vec::new())
    }

    async fn mark_delivered(&self, _id: i64) -> storage::Result<()> {
        Ok(())
    }

    async fn schedule_retry(
        &self,
        _id: i64,
        _attempts: i32,
        _next_retry_at: DateTime<Utc>,
    ) -> storage::Result<()> {
        Ok(())
    }

    async fn mark_failed(&self, _id: i64) -> storage::Result<()> {
        Ok(())
    }
}

#[async_trait]
impl CronJobRepo for FakeCronRepo {
    async fn create(&self, job: &CronJob) -> storage::Result<()> {
        self.jobs
            .lock()
            .expect("cron lock")
            .insert(job.id.clone(), job.clone());
        Ok(())
    }

    async fn get(&self, id: &str) -> storage::Result<Option<CronJob>> {
        Ok(self.jobs.lock().expect("cron lock").get(id).cloned())
    }

    async fn list_all(&self) -> storage::Result<Vec<CronJob>> {
        Ok(self
            .jobs
            .lock()
            .expect("cron lock")
            .values()
            .cloned()
            .collect())
    }

    async fn list_by_source(&self, source: CronJobSource) -> storage::Result<Vec<CronJob>> {
        Ok(self
            .jobs
            .lock()
            .expect("cron lock")
            .values()
            .filter(|job| job.source == source)
            .cloned()
            .collect())
    }

    async fn list_due(&self) -> storage::Result<Vec<CronJob>> {
        Ok(Vec::new())
    }

    async fn update_prompt(&self, id: &str, task_prompt: String) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.task_prompt = task_prompt;
        }
        Ok(())
    }

    async fn update_interval(&self, id: &str, interval_seconds: u64) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.interval_seconds = Some(interval_seconds);
        }
        Ok(())
    }

    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.next_run_at = next_run_at;
        }
        Ok(())
    }

    async fn set_state(&self, id: &str, state: CronJobState) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.state = state;
        }
        Ok(())
    }

    async fn record_run(
        &self,
        id: &str,
        run_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
    ) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.last_run_at = Some(run_at);
            job.last_status = Some(status.to_string());
            job.last_error = error.map(ToString::to_string);
        }
        Ok(())
    }

    async fn increment_repeat(&self, id: &str) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.repeat_completed += 1;
        }
        Ok(())
    }

    async fn record_result(&self, id: &str, result: &str) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.last_status = Some("completed".to_string());
            job.last_result = Some(result.to_string());
        }
        Ok(())
    }

    async fn complete_delegate_result(
        &self,
        id: &str,
        completed_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
        result: &str,
    ) -> storage::Result<()> {
        if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
            job.state = CronJobState::Completed;
            job.last_run_at = Some(completed_at);
            job.last_status = Some(status.to_string());
            job.last_error = error.map(ToString::to_string);
            job.last_result = Some(result.to_string());
            job.repeat_completed += 1;
        }
        Ok(())
    }

    async fn delete(&self, id: &str) -> storage::Result<()> {
        self.jobs.lock().expect("cron lock").remove(id);
        Ok(())
    }
}

struct SkillSyncChannel;

#[async_trait]
impl OutboundChannel for SkillSyncChannel {
    fn name(&self) -> &str {
        "skill-sync"
    }

    fn kind(&self) -> &'static str {
        "test"
    }

    fn accepts(&self, event_type: &str) -> bool {
        event_type == event_types::SKILL_SYNCED || event_type.starts_with("schedule.")
    }

    async fn deliver(&self, _event: &OutboundEvent) -> outbound::Result<()> {
        Err(OutboundError::Delivery("not used in emitter tests".into()))
    }
}

fn temp_workspace() -> PathBuf {
    let path = std::env::temp_dir().join(format!(
        "hivy-sandboxes-runtime-skill-sync-{}",
        Utc::now().timestamp_nanos_opt().unwrap_or_default()
    ));
    fs::create_dir_all(&path).expect("create temp workspace");
    path
}

fn test_emitter(outbox: Arc<FakeOutbox>) -> Arc<OutboundEmitter> {
    let registry = OutboundRegistry::new().with_channel(Arc::new(SkillSyncChannel));
    Arc::new(OutboundEmitter::new(
        outbox,
        Arc::new(RwLock::new(registry)),
    ))
}

fn skill_manage_test_tool(workspace: PathBuf, outbox: Arc<FakeOutbox>) -> Arc<dyn JsonTool> {
    let emitter = test_emitter(outbox);
    let ctx = ToolContext {
        gateway: None,
        cron_repo: None,
        event_repo: None,
        process_registry: None,
        mcp_registry: None,
        workspace_root: workspace,
        outbound_emitter: Some(emitter),
        agent_registry: Arc::new(AgentDefinitionRegistry::from_definition(Arc::new(
            test_agent_definition(),
        ))),
    };
    build_agent_tools(
        &[ToolSpec::SkillManage],
        &SessionId::from("C123-456.789"),
        &ctx,
    )
    .into_iter()
    .find(|tool| tool.definition().name == "skill_manage")
    .expect("skill_manage tool")
}

#[tokio::test]
async fn skill_manage_create_emits_complete_sync_snapshot() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    tool.call(json!({
            "action": "create",
            "name": "debug-deploys",
            "category": "engineering",
            "content": "---\nname: debug-deploys\ndescription: Debug deploy failures.\ntags: deploy, debug\n---\n# Debug\nCheck logs first."
        }))
        .await
        .expect("skill create");
    tool.call(json!({
        "action": "write_file",
        "name": "debug-deploys",
        "file_path": "references/errors.md",
        "file_content": "# Errors"
    }))
    .await
    .expect("supporting file write");

    let rows = outbox.rows.lock().expect("outbox lock");
    assert_eq!(rows.len(), 2);
    let (_, event_type, payload) = &rows[1];
    assert_eq!(event_type, event_types::SKILL_SYNCED);
    assert_eq!(payload["action"], "write_file");
    assert_eq!(payload["name"], "debug-deploys");
    assert_eq!(payload["source"], "unknown");
    assert_eq!(payload["description"], "Debug deploy failures.");
    assert_eq!(payload["files"]["references/errors.md"], "# Errors");
    assert!(payload["content"]
        .as_str()
        .expect("content string")
        .contains("# Debug"));

    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn skill_manage_failed_call_emits_no_sync_event() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    let result = tool
        .call(json!({
            "action": "write_file",
            "name": "missing-skill",
            "file_path": "references/errors.md",
            "content": "# Errors"
        }))
        .await;

    assert!(result.is_err());
    assert!(outbox.rows.lock().expect("outbox lock").is_empty());
    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn skill_manage_delete_emits_tombstone() {
    let workspace = temp_workspace();
    let outbox = Arc::new(FakeOutbox::default());
    let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

    tool.call(json!({
        "action": "create",
        "name": "debug-deploys",
        "content": "---\nname: debug-deploys\n---\n# Debug"
    }))
    .await
    .expect("skill create");
    tool.call(json!({
        "action": "create",
        "name": "deploy-ops",
        "content": "---\nname: deploy-ops\n---\n# Deploy ops"
    }))
    .await
    .expect("absorbed target create");
    tool.call(json!({
        "action": "delete",
        "name": "debug-deploys",
        "absorbed_into": "deploy-ops"
    }))
    .await
    .expect("skill delete");

    let rows = outbox.rows.lock().expect("outbox lock");
    assert_eq!(rows.len(), 3);
    let (_, event_type, payload) = &rows[2];
    assert_eq!(event_type, event_types::SKILL_SYNCED);
    assert_eq!(payload["action"], "delete");
    assert_eq!(payload["deleted"], true);
    assert_eq!(payload["absorbed_into"], "deploy-ops");
    assert!(payload.get("content").is_none());

    let _ = fs::remove_dir_all(workspace);
}

#[tokio::test]
async fn cron_create_update_pause_resume_cancel_emit_schedule_events() {
    let repo = Arc::new(FakeCronRepo::default());
    let outbox = Arc::new(FakeOutbox::default());
    let tool = cron_tool(
        repo,
        SessionId::from("C123-456.789"),
        Some(test_emitter(outbox.clone())),
    );

    let created = tool
        .call(json!({
            "action": "create",
            "task_prompt": "Check deploy health",
            "interval_seconds": 3600,
            "description": "Deploy health"
        }))
        .await
        .expect("create cron");
    let job_id = created["job_id"].as_str().expect("job id").to_string();
    tool.call(json!({"action": "update", "job_id": job_id, "task_prompt": "Check API health", "interval_seconds": 7200}))
            .await
            .expect("update cron");
    tool.call(json!({"action": "pause", "job_id": job_id}))
        .await
        .expect("pause cron");
    tool.call(json!({"action": "resume", "job_id": job_id}))
        .await
        .expect("resume cron");
    tool.call(json!({"action": "cancel", "job_id": job_id}))
        .await
        .expect("cancel cron");

    let rows = outbox.rows.lock().expect("outbox lock");
    let event_types: Vec<_> = rows
        .iter()
        .map(|(_, event_type, _)| event_type.as_str())
        .collect();
    assert_eq!(
        event_types,
        vec![
            event_types::SCHEDULE_CREATED,
            event_types::SCHEDULE_UPDATED,
            event_types::SCHEDULE_PAUSED,
            event_types::SCHEDULE_RESUMED,
            event_types::SCHEDULE_CANCELLED,
        ]
    );
    assert_eq!(rows[0].2["source"], "cron");
    assert_eq!(rows[0].2["origin"], "tool");
    assert_eq!(rows[0].2["task_prompt"], "Check deploy health");
}

#[tokio::test]
async fn wake_jobs_do_not_emit_schedule_events() {
    let now = Utc::now();
    let outbox = Arc::new(FakeOutbox::default());
    let job = CronJob {
        id: "wake-1".to_string(),
        description: "Wake".to_string(),
        channel: "C123".to_string(),
        task_prompt: "Wake up".to_string(),
        cron_expression: None,
        interval_seconds: None,
        repeat_count: Some(1),
        repeat_completed: 0,
        state: CronJobState::Active,
        source: CronJobSource::Cron,
        next_run_at: now,
        last_run_at: None,
        last_status: None,
        last_error: None,
        delegated_session_id: None,
        session_continuation_id: Some("C123-456.789".to_string()),
        created_at: now,
        created_by_session: "C123-456.789".to_string(),
        agent_name: None,
        last_result: None,
    };
    emit_schedule_event(
        Some(test_emitter(outbox.clone())),
        event_types::SCHEDULE_CREATED,
        &job,
        &SessionId::from("C123-456.789"),
        "tool",
        None,
    )
    .await;
    assert!(outbox.rows.lock().expect("outbox lock").is_empty());
}
