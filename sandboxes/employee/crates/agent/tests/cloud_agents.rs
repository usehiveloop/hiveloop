use std::collections::HashMap;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;

use agent::cloud_agents::{
    format_cloud_agents_prompt, strip_cloud_agents_block, CloudAgent, CloudAgentConfig,
    CloudAgentService,
};
use agent::rig_tool_registry::{build_agent_tools, ToolContext};
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::IntoResponse;
use axum::routing::{get, post};
use axum::{Json, Router};
use domain::{SessionId, ToolSpec};
use serde_json::{json, Value};
use tokio::net::TcpListener;
use tokio::sync::Mutex;

#[tokio::test]
async fn launch_sends_prompt_as_brief_and_description_in_metadata() {
    let fake = FakeControlPlane::spawn().await;
    let service = CloudAgentService::new(fake.config());

    let result = service
        .launch_task(
            "agent-code",
            "Implement cache fix",
            "Full standalone task prompt only",
            &SessionId::from_slack("C0B2EUVR5B6", "1778326607.193629"),
        )
        .await
        .expect("launch task");

    assert_eq!(result["task_id"], "task-2");

    let launches = fake.state.launches.lock().await;
    assert_eq!(launches.len(), 1);
    let body = &launches[0];
    assert_eq!(body["brief"], "Full standalone task prompt only");
    assert_eq!(body["parent_conversation_type"], "agent_conversation");
    assert_eq!(
        body["parent_conversation_id"],
        "C0B2EUVR5B6-1778326607.193629"
    );
    assert_eq!(body["metadata"]["description"], "Implement cache fix");
    assert_eq!(
        body["metadata"]["session_id"],
        "C0B2EUVR5B6-1778326607.193629"
    );
    assert_eq!(body["metadata"]["channel"], "C0B2EUVR5B6");
    assert_eq!(body["metadata"]["thread_ts"], "1778326607.193629");
    assert_eq!(body["metadata"]["source"], "employee_bridge");
}

#[tokio::test]
async fn tools_flow_end_to_end_against_fake_control_plane() {
    let fake = FakeControlPlane::spawn().await;
    let service = Arc::new(CloudAgentService::new(fake.config()));
    service.discover().await.expect("discover cloud agents");

    let session_id = SessionId::from_slack("C0B2EUVR5B6", "1778326607.193629");
    let tools = build_agent_tools(
        &[
            ToolSpec::CloudAgentLaunchTask,
            ToolSpec::CloudAgentListTasks,
            ToolSpec::CloudAgentTaskStatus,
            ToolSpec::CloudAgentTaskSendMessage,
            ToolSpec::CloudAgentTaskTerminate,
        ],
        &session_id,
        &ToolContext {
            gateway: None,
            cron_repo: None,
            process_registry: None,
            mcp_registry: None,
            workspace_root: PathBuf::from("/tmp"),
            cloud_agents: Some(service),
            outbound_emitter: None,
            slack_channels: Vec::new(),
        },
    );
    let find_tool = |name: &str| {
        tools
            .iter()
            .find(|tool| tool.definition().name == name)
            .expect("tool exists")
            .clone()
    };

    let launch = find_tool("cloud_agent_launch_task")
        .call(json!({
            "agent_id": "agent-code",
            "description": "Build feature",
            "prompt": "Implement the feature and report back."
        }))
        .await
        .expect("launch");
    assert_eq!(launch["task_id"], "task-2");

    let list = find_tool("cloud_agent_list_tasks")
        .call(json!({"agent_id": "agent-code"}))
        .await
        .expect("list");
    assert_eq!(list["tasks"].as_array().unwrap().len(), 2);
    assert_eq!(list["tasks"][0]["description"], "Seed task");

    let status = find_tool("cloud_agent_task_status")
        .call(json!({"task_id": "task-2"}))
        .await
        .expect("status");
    assert_eq!(status["status"], "running");
    assert_eq!(status["description"], "Build feature");

    let message = find_tool("cloud_agent_task_send_message")
        .call(json!({"task_id": "task-2", "message": "Please focus on tests too."}))
        .await
        .expect("message");
    assert_eq!(message["success"], true);

    let terminated = find_tool("cloud_agent_task_terminate")
        .call(json!({"task_id": "task-2", "reason": "User cancelled"}))
        .await
        .expect("terminate");
    assert_eq!(terminated["status"], "completed");

    let final_status = find_tool("cloud_agent_task_status")
        .call(json!({"task_id": "task-2"}))
        .await
        .expect("final status");
    assert_eq!(final_status["status"], "completed");
}

#[test]
fn prompt_is_compact_and_strip_is_idempotent() {
    let agents: Vec<CloudAgent> = serde_json::from_value(json!([{
        "id": "agent-code",
        "name": "Builder",
        "system_prompt": "SECRET SYSTEM PROMPT",
        "tools": [{"name": "secret_internal_tool"}],
        "skills": [{"name": "internal"}],
        "recent_tasks": [{
            "id": "task-1",
            "brief": "FULL TASK PROMPT SHOULD NOT APPEAR",
            "metadata": {
                "description": "Seed task",
                "private": "SECRET METADATA"
            },
            "created_at": "2026-05-09T10:00:00Z",
            "recent_events": [{
                "type": "ToolCall",
                "created_at": "2026-05-09T10:01:00Z",
                "data": {"message": "Working"}
            }]
        }]
    }]))
    .unwrap();

    let prompt = format_cloud_agents_prompt(&agents);
    assert!(prompt.contains("## Cloud Agents"));
    assert!(prompt.contains("<cloud_agent_context>"));
    assert!(prompt.contains("</cloud_agent_context>"));
    assert!(prompt.contains("Builder (agent-code)"));
    assert!(prompt.contains("task-1 - Seed task"));
    assert!(prompt.contains("ToolCall"));
    assert!(!prompt.contains("SECRET SYSTEM PROMPT"));
    assert!(!prompt.contains("FULL TASK PROMPT SHOULD NOT APPEAR"));
    assert!(!prompt.contains("SECRET METADATA"));
    assert!(!prompt.contains("secret_internal_tool"));
    assert!(!prompt.contains("internal"));

    let wrapped = format!("base\n\n{prompt}\n\nsuffix");
    let stripped = strip_cloud_agents_block(&strip_cloud_agents_block(&wrapped));
    assert!(stripped.contains("## Cloud Agents"));
    assert!(stripped.contains("When to create a cloud agent task"));
    assert!(!stripped.contains("<cloud_agent_context>"));
    assert!(!stripped.contains("Builder (agent-code)"));
    assert!(stripped.starts_with("base"));
    assert!(stripped.ends_with("suffix"));
}

struct FakeControlPlane {
    addr: SocketAddr,
    state: Arc<FakeState>,
}

impl FakeControlPlane {
    async fn spawn() -> Self {
        let state = Arc::new(FakeState::default());
        state.seed().await;

        let app = Router::new()
            .route(
                "/internal/employees/:employee_id/cloud-agents/",
                get(list_agents),
            )
            .route(
                "/internal/employees/:employee_id/cloud-agents/:agent_id/tasks",
                get(list_tasks).post(create_task),
            )
            .route(
                "/internal/employees/:employee_id/cloud-agents/:agent_id/tasks/:task_id",
                get(get_task).post(terminate_task),
            )
            .route(
                "/internal/employees/:employee_id/cloud-agents/:agent_id/tasks/:task_id/message",
                post(send_message),
            )
            .with_state(state.clone());
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        Self { addr, state }
    }

    fn config(&self) -> CloudAgentConfig {
        CloudAgentConfig {
            employee_id: "emp-1".to_string(),
            control_plane_url: format!("http://{}", self.addr),
            bridge_api_key: "cloud-agent-test-token".to_string(),
        }
    }
}

#[derive(Default)]
struct FakeState {
    agents: Mutex<Vec<Value>>,
    tasks: Mutex<HashMap<String, Vec<Value>>>,
    launches: Mutex<Vec<Value>>,
}

impl FakeState {
    async fn seed(&self) {
        let seeded_task = json!({
            "id": "task-1",
            "cloud_agent_id": "agent-code",
            "created_at": "2026-05-09T10:00:00Z",
            "metadata": {"description": "Seed task"},
            "recent_events": [{"type": "AgentStarted", "created_at": "2026-05-09T10:00:01Z", "data": {"message": "started"}}]
        });
        self.agents.lock().await.push(json!({
            "id": "agent-code",
            "name": "Builder",
            "system_prompt": "not prompt-visible",
            "tools": [{"name": "bash"}],
            "skills": [],
            "recent_tasks": [seeded_task.clone()]
        }));
        self.tasks
            .lock()
            .await
            .insert("agent-code".to_string(), vec![seeded_task]);
    }
}

async fn list_agents(State(state): State<Arc<FakeState>>, headers: HeaderMap) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    let agents = state.agents.lock().await.clone();
    Json(json!({ "cloud_agents": agents })).into_response()
}

async fn list_tasks(
    State(state): State<Arc<FakeState>>,
    Path((_employee_id, agent_id)): Path<(String, String)>,
    Query(_query): Query<HashMap<String, String>>,
    headers: HeaderMap,
) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    let tasks = state
        .tasks
        .lock()
        .await
        .get(&agent_id)
        .cloned()
        .unwrap_or_default();
    Json(json!({ "data": tasks, "has_more": false, "next_cursor": null })).into_response()
}

async fn create_task(
    State(state): State<Arc<FakeState>>,
    Path((_employee_id, agent_id)): Path<(String, String)>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    state.launches.lock().await.push(body.clone());
    let task_id = format!("task-{}", state.launches.lock().await.len() + 1);
    let task = json!({
        "id": task_id,
        "cloud_agent_id": agent_id,
        "created_at": "2026-05-09T10:05:00Z",
        "metadata": body["metadata"].clone(),
        "recent_events": [{"type": "AgentStarted", "created_at": "2026-05-09T10:05:01Z", "data": {"message": "started"}}]
    });
    state
        .tasks
        .lock()
        .await
        .entry(agent_id)
        .or_default()
        .push(task);
    Json(json!({ "task_id": task_id, "message": "created" })).into_response()
}

async fn get_task(
    State(state): State<Arc<FakeState>>,
    Path((_employee_id, agent_id, task_id)): Path<(String, String, String)>,
    headers: HeaderMap,
) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    let task = state
        .tasks
        .lock()
        .await
        .get(&agent_id)
        .and_then(|tasks| tasks.iter().find(|task| task["id"] == task_id).cloned());
    match task {
        Some(task) => Json(task).into_response(),
        None => StatusCode::NOT_FOUND.into_response(),
    }
}

async fn send_message(
    State(state): State<Arc<FakeState>>,
    Path((_employee_id, agent_id, task_id)): Path<(String, String, String)>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    append_event(
        &state,
        &agent_id,
        &task_id,
        json!({"type": "CoordinatorMessage", "created_at": "2026-05-09T10:06:00Z", "data": body}),
    )
    .await;
    Json(json!({ "message": "sent" })).into_response()
}

async fn terminate_task(
    State(state): State<Arc<FakeState>>,
    Path((_employee_id, agent_id, task_id)): Path<(String, String, String)>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> impl IntoResponse {
    if let Err(status) = validate_auth(&headers) {
        return status.into_response();
    }
    append_event(
        &state,
        &agent_id,
        &task_id,
        json!({"type": "ConversationEnded", "created_at": "2026-05-09T10:07:00Z", "data": body}),
    )
    .await;
    Json(json!({ "message": "terminated" })).into_response()
}

async fn append_event(state: &FakeState, agent_id: &str, task_id: &str, event: Value) {
    let mut tasks_by_agent = state.tasks.lock().await;
    if let Some(tasks) = tasks_by_agent.get_mut(agent_id) {
        if let Some(task) = tasks.iter_mut().find(|task| task["id"] == task_id) {
            if let Some(events) = task["recent_events"].as_array_mut() {
                events.insert(0, event);
            }
        }
    }
}

fn validate_auth(headers: &HeaderMap) -> Result<(), StatusCode> {
    match headers
        .get("authorization")
        .and_then(|value| value.to_str().ok())
    {
        Some("Bearer cloud-agent-test-token") => Ok(()),
        _ => Err(StatusCode::UNAUTHORIZED),
    }
}
