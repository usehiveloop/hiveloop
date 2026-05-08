use std::sync::Arc;

use adk_rust::prelude::*;
use adk_rust::session::{InMemorySessionService, SessionService};
use chrono::Utc;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::{ConfigStore, SessionId};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;
use tools::ToolBuildContext;

use crate::tool_registry::{build_agent_tools, ToolContext as AgentToolContext};

const TOOL_NAME: &str = "delegate";
const TOOL_DESCRIPTION: &str =
    "Spawn subagents to work on tasks in isolated conversations. \
     Each subagent gets its own tools and context. \
     Only final summaries are returned — intermediate tool results never enter your context.\n\n\
     WHEN TO USE delegate:\n\
     - Reasoning-heavy subtasks (local code exploration, web research)\n\
     - Tasks that would flood your context with intermediate data\n\
     - Parallel independent workstreams (e.g. analyze issues AND check health)\n\n\
     - Short-running low resource work like exploring local files, research that take just a few minutes to complete. \n\n\
     - Simple scripts writing, investigations \n\n\
     WHEN NOT TO USE:\n\
     - Single tool call — call the tool directly\n\
     - Long-running resource intensive operations like coding, media generation, deep research, browser use - use available cloud agents instead \n\n\
     - Long-running low resource work like exploring local files, small research that should complete after your response — set run_in_background=true instead\n\n\
     Subagents CANNOT use: delegate, cron, post_status_update, post_to_channel.\n\
     Pass relevant info (file paths, errors, constraints) via the context field per task.";

const SUBAGENT_SYSTEM_PROMPT: &str =
    "You are a focused subagent working on a specific delegated task. \
     YOUR TASK is below. Complete it using the tools available to you. \
     When finished, provide a concise summary of what you did, found, and any files created.\
     \n\nDo NOT ask clarifying questions — work with the information given.";

const BLOCKED_TOOL_NAMES: &[&str] = &["cron", "delegate", "post_status_update", "post_to_channel"];

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct DelegateTask {
    pub goal: String,
    #[serde(default)]
    pub context: Option<String>,
    #[serde(default)]
    pub toolsets: Option<Vec<String>>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct DelegateArgs {
    pub tasks: Vec<DelegateTask>,
    #[serde(default)]
    pub run_in_background: bool,
}

pub struct DelegateContext {
    pub config: ConfigStore,
    pub session_service: Arc<dyn SessionService>,
    pub tool_context: Arc<ToolBuildContext>,
    pub agent_tool_context: AgentToolContext,
}

pub struct DelegateTool {
    ctx: Arc<DelegateContext>,
    session_id: SessionId,
    cron_repo: Arc<dyn CronJobRepo>,
}

impl DelegateTool {
    pub fn new(
        ctx: Arc<DelegateContext>,
        session_id: SessionId,
        cron_repo: Arc<dyn CronJobRepo>,
    ) -> Self {
        Self { ctx, session_id, cron_repo }
    }

    pub fn into_adk_tool(self) -> Arc<dyn Tool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_tool_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<DelegateArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: DelegateArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;

        if parsed.tasks.is_empty() {
            return Err(AdkError::tool("tasks must not be empty"));
        }
        if parsed.tasks.len() > 10 {
            return Err(AdkError::tool("maximum 10 tasks per delegate call"));
        }

        if parsed.run_in_background {
            self.run_background(parsed).await
        } else {
            self.run_parallel(parsed).await
        }
    }

    async fn run_background(&self, parsed: DelegateArgs) -> Result<Value> {
        let mut job_ids = Vec::new();
        let channel = self.session_id.as_str().split_once('-')
            .map(|(c, _)| c.to_string())
            .unwrap_or_else(|| self.session_id.as_str().to_string());

        for task in &parsed.tasks {
            let job_id = format!("cron-bg-{}", Utc::now().timestamp_millis());
            let delegated_session_id = format!("{}-delegate-{}", channel, job_id);
            let job = CronJob {
                id: job_id.clone(),
                description: format!("delegate: {}", &task.goal[..task.goal.len().min(80)]),
                channel: channel.clone(),
                task_prompt: format!(
                    "You are running a background task created via delegate.\n\nYOUR TASK:\n{}",
                    task.goal
                ),
                cron_expression: None,
                interval_seconds: Some(0),
                repeat_count: None,
                repeat_completed: 0,
                state: CronJobState::Active,
                source: CronJobSource::Delegate,
                next_run_at: Utc::now(),
                last_run_at: None,
                last_status: None,
                last_error: None,
                delegated_session_id: Some(delegated_session_id),
                session_continuation_id: None,
                created_at: Utc::now(),
                created_by_session: self.session_id.as_str().to_string(),
            };
            self.cron_repo.create(&job).await
                .map_err(|e| AdkError::tool(format!("create bg job: {e}")))?;
            job_ids.push(job_id);
        }

        Ok(serde_json::json!({
            "mode": "background",
            "job_ids": job_ids,
            "message": format!("{} background task(s) scheduled", job_ids.len()),
        }))
    }

    async fn run_parallel(&self, parsed: DelegateArgs) -> Result<Value> {
        let snapshot = self.ctx.config.snapshot();
        let model_config = describe_model(&snapshot.model);
        let all_tools = tools::build_builtin_tools(&snapshot.tools, &self.ctx.tool_context);
        let agent_tools = build_agent_tools(
            &snapshot.tools,
            &self.session_id,
            &self.ctx.agent_tool_context,
        );

        let mut futures = Vec::new();
        for task in &parsed.tasks {
            let goal = task.goal.clone();
            let context = task.context.clone().unwrap_or_default();
            let model_config = model_config.clone();
            let mut child_tools: Vec<Arc<dyn Tool>> = Vec::new();
            for tool in &all_tools {
                child_tools.push(tool.clone());
            }
            for tool in &agent_tools {
                let name = tool.name().to_string();
                if BLOCKED_TOOL_NAMES.contains(&name.as_str()) {
                    continue;
                }
                child_tools.push(tool.clone());
            }

            let svc = Arc::new(InMemorySessionService::new());
            let child_session_id = format!("delegate-{}", Utc::now().timestamp_millis());

            futures.push(tokio::spawn(async move {
                run_child_agent(goal, context, model_config, child_tools, svc, &child_session_id).await
            }));
        }

        let results = futures::future::join_all(futures).await;
        let mut output: Vec<Value> = Vec::new();
        for res in results {
            match res {
                Ok(Ok(text)) => output.push(serde_json::json!({"result": text})),
                Ok(Err(e)) => output.push(serde_json::json!({"error": e})),
                Err(e) => output.push(serde_json::json!({"error": format!("task panicked: {e}")})),
            }
        }

        Ok(serde_json::json!({
            "mode": "parallel",
            "results": output,
            "total": output.len(),
        }))
    }
}

fn describe_model(config: &domain::ModelConfig) -> String {
    match config {
        domain::ModelConfig::OpenaiCompatible { model_id, .. } => model_id.clone(),
    }
}

async fn run_child_agent(
    goal: String,
    context: String,
    model_id: String,
    tools: Vec<Arc<dyn Tool>>,
    svc: Arc<dyn SessionService>,
    session_id: &str,
) -> std::result::Result<String, String> {
    let api_key = std::env::var("OPENROUTER_API_KEY")
        .map_err(|_| "OPENROUTER_API_KEY not set".to_string())?;
    let cfg = OpenAIConfig::compatible(api_key, "https://openrouter.ai/api/v1", &model_id);
    let client = OpenAIClient::new(cfg)
        .map_err(|e| format!("OpenAIClient init: {e}"))?;

    let model = crate::streaming_fix::AccumulatingStreamLlm::wrap(Arc::new(client));

    let mut prompt = format!("{}\n\nYOUR TASK:\n{}", SUBAGENT_SYSTEM_PROMPT, goal);
    if !context.is_empty() {
        prompt.push_str(&format!("\n\nCONTEXT:\n{}", context));
    }

    let mut builder = LlmAgentBuilder::new("subagent")
        .instruction(prompt)
        .model(model);
    for tool in &tools {
        builder = builder.tool(tool.clone());
    }

    let agent: Arc<dyn Agent> = Arc::new(
        builder.build().map_err(|e| format!("build: {e}"))?,
    );

    let runner = Runner::builder()
        .app_name("employee-bridge")
        .agent(agent)
        .session_service(svc)
        .build()
        .map_err(|e| format!("runner build: {e}"))?;

    let user_id = adk_rust::UserId::new("runtime")
        .map_err(|e| format!("user_id: {e}"))?;
    let adk_sid = adk_rust::SessionId::new(session_id)
        .map_err(|e| format!("session_id: {e}"))?;

    let content = Content::new("user").with_text(goal);
    let mut stream = runner.run(user_id, adk_sid, content)
        .await
        .map_err(|e| format!("runner.run: {e}"))?;

    use futures::StreamExt;
    let mut result = String::new();
    while let Some(Ok(evt)) = stream.next().await {
        if !evt.llm_response.partial {
            continue;
        }
        if let Some(c) = evt.llm_response.content {
            for part in &c.parts {
                if let Part::Text { text } = part {
                    result.push_str(text);
                }
            }
        }
    }
    Ok(result)
}
