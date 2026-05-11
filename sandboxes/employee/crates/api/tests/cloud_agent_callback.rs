use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use agent::cloud_agents::{CloudTaskIndex, TaskIndexEntry};
use async_trait::async_trait;
use axum::{
    body::{to_bytes, Body},
    http::{header, Method, Request, StatusCode},
};
use chrono::{DateTime, Utc};
use domain::{
    AgentDefinition, AgentMeta, ConfigStore, EventKind, Limits, ModelConfig, Session, SessionEvent,
    SessionId, SessionStatus,
};
use serde_json::{json, Map, Value};
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo};
use tokio::sync::mpsc;
use tower::ServiceExt;

#[tokio::test]
async fn callback_requires_bearer_auth() {
    let fixture = Fixture::new(None).await;
    let response = fixture
        .router
        .oneshot(callback_request(None, json!({})))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn callback_persists_event_and_publishes_to_active_stream() {
    let fixture = Fixture::new(None).await;
    let stream_id = fixture.broker.create_stream().await;
    fixture
        .broker
        .register_session("http-conversation", &stream_id)
        .await;

    let body = json!({
        "task_id": "task-1",
        "agent_id": "agent-1",
        "event_type": "ProgressUpdate",
        "event_id": "event-1",
        "timestamp": "2026-05-10T09:00:00Z",
        "metadata": {"session_id": "http-conversation"},
        "data": {"summary": "halfway there"}
    });

    let response = fixture
        .router
        .clone()
        .oneshot(callback_request(Some("secret"), body))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::ACCEPTED);
    let events = fixture.events.events.lock().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].session_id.as_str(), "http-conversation");
    assert_eq!(events[0].kind, EventKind::CloudAgentEvent);
    assert_eq!(events[0].payload["source"], "cloud_agent_callback");
    assert_eq!(events[0].payload["event_id"], "event-1");

    let (history, _) = fixture
        .broker
        .subscribe(&stream_id)
        .await
        .expect("stream exists");
    assert_eq!(history.len(), 1);
    assert_eq!(history[0].event, "cloud_agent_event");
    assert_eq!(history[0].payload["data"]["summary"], "halfway there");
}

#[tokio::test]
async fn callback_is_idempotent_by_event_id() {
    let fixture = Fixture::new(None).await;
    let body = json!({
        "task_id": "task-1",
        "agent_id": "agent-1",
        "event_type": "ProgressUpdate",
        "event_id": "event-dup",
        "timestamp": "2026-05-10T09:00:00Z",
        "metadata": {"session_id": "http-conversation"},
        "data": {"summary": "only once"}
    });

    let first = fixture
        .router
        .clone()
        .oneshot(callback_request(Some("secret"), body.clone()))
        .await
        .expect("first response");
    let second = fixture
        .router
        .clone()
        .oneshot(callback_request(Some("secret"), body))
        .await
        .expect("second response");

    assert_eq!(first.status(), StatusCode::ACCEPTED);
    assert_eq!(second.status(), StatusCode::OK);
    let duplicate_response: Value = response_json(second).await;
    assert_eq!(duplicate_response["accepted"], false);
    assert_eq!(duplicate_response["duplicate"], true);
    assert_eq!(fixture.events.events.lock().unwrap().len(), 1);
}

#[tokio::test]
async fn callback_routes_by_task_index_when_metadata_has_no_session_id() {
    let index = Arc::new(CloudTaskIndex::default());
    let mut metadata = Map::new();
    metadata.insert(
        "session_id".to_string(),
        Value::String("http-conversation".to_string()),
    );
    index
        .upsert_task(TaskIndexEntry {
            task_id: "task-indexed".to_string(),
            agent_id: "agent-1".to_string(),
            agent_name: Some("Code Agent".to_string()),
            description: Some("test".to_string()),
            created_at: None,
            recent_events: Vec::new(),
            metadata,
        })
        .await;
    let fixture = Fixture::new(Some(index)).await;

    let response = fixture
        .router
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-indexed",
                "agent_id": "agent-1",
                "event_type": "ProgressUpdate",
                "event_id": "event-indexed",
                "timestamp": "2026-05-10T09:00:00Z",
                "metadata": {},
                "data": {"summary": "found by index"}
            }),
        ))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::ACCEPTED);
    let events = fixture.events.events.lock().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].session_id.as_str(), "http-conversation");
}

#[tokio::test]
async fn terminal_and_error_callbacks_update_session_status() {
    let fixture = Fixture::new(None).await;

    let completed = fixture
        .router
        .clone()
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-1",
                "agent_id": "agent-1",
                "event_type": "ConversationEnded",
                "event_id": "event-complete",
                "timestamp": "2026-05-10T09:00:00Z",
                "metadata": {"session_id": "http-conversation"},
                "data": {"status": "done"}
            }),
        ))
        .await
        .expect("completed response");
    assert_eq!(completed.status(), StatusCode::ACCEPTED);
    assert_eq!(
        fixture.session_status("http-conversation"),
        Some(SessionStatus::Completed)
    );

    let errored = fixture
        .router
        .clone()
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-1",
                "agent_id": "agent-1",
                "event_type": "AgentError",
                "event_id": "event-error",
                "timestamp": "2026-05-10T09:01:00Z",
                "metadata": {"session_id": "http-conversation"},
                "data": {"message": "boom"}
            }),
        ))
        .await
        .expect("error response");
    assert_eq!(errored.status(), StatusCode::ACCEPTED);
    assert_eq!(
        fixture.session_status("http-conversation"),
        Some(SessionStatus::Errored)
    );
    let events = fixture.events.events.lock().unwrap();
    assert_eq!(events.last().unwrap().kind, EventKind::CloudAgentEvent);
    assert_eq!(events.last().unwrap().payload["status"], "errored");
}

struct Fixture {
    router: axum::Router,
    sessions: Arc<MemorySessionRepo>,
    events: Arc<MemoryEventRepo>,
    broker: Arc<api::HttpStreamBroker>,
}

impl Fixture {
    async fn new(cloud_task_index: Option<Arc<CloudTaskIndex>>) -> Self {
        let config = ConfigStore::new(test_definition());
        let config_repo = Arc::new(MemoryConfigRepo::default());
        let sessions = Arc::new(MemorySessionRepo::default());
        sessions.insert(session("http-conversation"));
        let events = Arc::new(MemoryEventRepo::default());
        let (tx, _rx) = mpsc::channel(1);
        let broker = Arc::new(api::HttpStreamBroker::new());
        let state = api::ApiState::new(
            config,
            config_repo,
            sessions.clone(),
            events.clone(),
            "secret".to_string(),
            Arc::new(SkillWriter::new(test_workspace())),
            Some(api::HttpGatewayState {
                inbound_sink: tx,
                broker: broker.clone(),
            }),
            None,
            cloud_task_index,
        );
        Self {
            router: api::build_router(state),
            sessions,
            events,
            broker,
        }
    }

    fn session_status(&self, session_id: &str) -> Option<SessionStatus> {
        self.sessions
            .sessions
            .lock()
            .unwrap()
            .get(session_id)
            .map(|session| session.status)
    }
}

fn callback_request(token: Option<&str>, body: Value) -> Request<Body> {
    let mut builder = Request::builder()
        .method(Method::POST)
        .uri("/gateway/cloud-agents/callback")
        .header(header::CONTENT_TYPE, "application/json");
    if let Some(token) = token {
        builder = builder.header(header::AUTHORIZATION, format!("Bearer {token}"));
    }
    builder.body(Body::from(body.to_string())).unwrap()
}

async fn response_json(response: axum::response::Response) -> Value {
    let bytes = to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("read body");
    serde_json::from_slice(&bytes).expect("json body")
}

fn session(id: &str) -> Session {
    let now = Utc::now();
    Session {
        id: SessionId::from(id),
        channel: "http".to_string(),
        thread_ts: "conversation".to_string(),
        agent_session_id: id.to_string(),
        status: SessionStatus::Active,
        created_at: now,
        last_activity_at: now,
    }
}

fn test_definition() -> AgentDefinition {
    AgentDefinition {
        agent: AgentMeta {
            name: "Test".to_string(),
            description: String::new(),
            system_prompt: "test".to_string(),
        },
        model: ModelConfig::OpenaiCompatible {
            base_url: "http://localhost".to_string(),
            model_id: "test".to_string(),
            api_key_env: "TEST_API_KEY".to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        },
        multimodal_model: None,
        limits: Limits::default(),
        context: Default::default(),
        tools: Vec::new(),
        mcp_servers: Vec::new(),
        skills: Vec::new(),
        subagents: Vec::new(),
        slack: Default::default(),
        outbound_channels: Vec::new(),
    }
}

fn test_workspace() -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "employee-api-test-{}",
        Utc::now().timestamp_nanos_opt().unwrap_or_default()
    ))
}

#[derive(Default)]
struct MemoryConfigRepo {
    definition: Mutex<Option<AgentDefinition>>,
}

#[async_trait]
impl ConfigRepo for MemoryConfigRepo {
    async fn load(&self) -> storage::Result<Option<AgentDefinition>> {
        Ok(self.definition.lock().unwrap().clone())
    }

    async fn upsert(&self, def: &AgentDefinition) -> storage::Result<()> {
        *self.definition.lock().unwrap() = Some(def.clone());
        Ok(())
    }
}

#[derive(Default)]
struct MemorySessionRepo {
    sessions: Mutex<HashMap<String, Session>>,
}

impl MemorySessionRepo {
    fn insert(&self, session: Session) {
        self.sessions
            .lock()
            .unwrap()
            .insert(session.id.as_str().to_string(), session);
    }
}

#[async_trait]
impl SessionRepo for MemorySessionRepo {
    async fn get(&self, id: &SessionId) -> storage::Result<Option<Session>> {
        Ok(self.sessions.lock().unwrap().get(id.as_str()).cloned())
    }

    async fn create(&self, session: &Session) -> storage::Result<()> {
        self.insert(session.clone());
        Ok(())
    }

    async fn touch(&self, id: &SessionId, at: DateTime<Utc>) -> storage::Result<()> {
        if let Some(session) = self.sessions.lock().unwrap().get_mut(id.as_str()) {
            session.last_activity_at = at;
        }
        Ok(())
    }

    async fn set_status(&self, id: &SessionId, status: SessionStatus) -> storage::Result<()> {
        if let Some(session) = self.sessions.lock().unwrap().get_mut(id.as_str()) {
            session.status = status;
        }
        Ok(())
    }

    async fn list(
        &self,
        _cursor: Option<DateTime<Utc>>,
        _status: Option<SessionStatus>,
        _limit: u32,
    ) -> storage::Result<Vec<Session>> {
        Ok(self.sessions.lock().unwrap().values().cloned().collect())
    }
}

#[derive(Default)]
struct MemoryEventRepo {
    events: Mutex<Vec<SessionEvent>>,
}

#[async_trait]
impl EventRepo for MemoryEventRepo {
    async fn append(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: Value,
    ) -> storage::Result<i64> {
        let mut events = self.events.lock().unwrap();
        let id = events.len() as i64 + 1;
        let seq = events
            .iter()
            .filter(|event| event.session_id == *session_id)
            .count() as i64
            + 1;
        events.push(SessionEvent {
            id,
            session_id: session_id.clone(),
            seq,
            kind,
            payload,
            created_at: Utc::now(),
        });
        Ok(id)
    }

    async fn list_recent(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> storage::Result<Vec<SessionEvent>> {
        let mut events: Vec<_> = self
            .events
            .lock()
            .unwrap()
            .iter()
            .filter(|event| event.session_id == *session_id)
            .cloned()
            .collect();
        events.reverse();
        events.truncate(limit as usize);
        Ok(events)
    }

    async fn list_chronological(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> storage::Result<Vec<SessionEvent>> {
        let mut events: Vec<_> = self
            .events
            .lock()
            .unwrap()
            .iter()
            .filter(|event| event.session_id == *session_id)
            .cloned()
            .collect();
        events.truncate(limit as usize);
        Ok(events)
    }
}
