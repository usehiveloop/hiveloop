use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use agent::cloud_agents::CloudTaskIndex;
use async_trait::async_trait;
use axum::{
    body::{to_bytes, Body},
    http::{header, Method, Request, StatusCode},
};
use chrono::{DateTime, Utc};
use domain::{
    AgentDefinition, AgentMeta, ConfigStore, EventKind, Limits, ModelConfig, OutboundChannelKind,
    OutboundChannelSpec, Session, SessionEvent, SessionId, SessionStatus,
};
use serde_json::{json, Value};
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo};
use tokio::sync::mpsc;
use tower::ServiceExt;

#[tokio::test]
async fn put_config_reloads_outbound_channels() {
    let config = ConfigStore::new(test_definition());
    let config_repo = Arc::new(MemoryConfigRepo::default());
    let sessions = Arc::new(MemorySessionRepo::default());
    let events = Arc::new(MemoryEventRepo::default());
    let reloader = Arc::new(RecordingOutboundReloader::default());
    let (tx, _rx) = mpsc::channel(1);
    let broker = Arc::new(api::HttpStreamBroker::new());
    let state = api::ApiState::new(
        config,
        config_repo,
        sessions,
        events,
        "callback-test-token".to_string(),
        Arc::new(SkillWriter::new(test_workspace())),
        Some(api::HttpGatewayState {
            inbound_sink: tx,
            broker,
        }),
        None,
        Some(reloader.clone()),
        None,
        None,
    );
    let router = api::build_router(state);

    let mut definition = test_definition();
    definition.outbound_channels = vec![OutboundChannelSpec {
        name: "control-plane-memory".to_string(),
        kind: OutboundChannelKind::Webhook {
            url: "https://api.usehiveloop.com/internal/webhooks/employee/sandbox-id".to_string(),
            secret_env: "RUNTIME_SECRET".to_string(),
            extra_headers: Default::default(),
        },
        event_filter: None,
    }];
    let response = router
        .oneshot(
            Request::builder()
                .method(Method::PUT)
                .uri("/config")
                .header(header::AUTHORIZATION, "Bearer callback-test-token")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::to_vec(&definition).unwrap()))
                .unwrap(),
        )
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(
        reloader.reloads.lock().unwrap().as_slice(),
        &[vec!["control-plane-memory".to_string()]]
    );
}

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

    let body = json!({
        "task_id": "task-1",
        "agent_id": "agent-1",
        "session_id": "http-conversation",
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
        .subscribe(&fixture.http_stream_id)
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
        "session_id": "http-conversation",
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
    let (history, _) = fixture
        .broker
        .subscribe(&fixture.http_stream_id)
        .await
        .expect("stream exists");
    assert_eq!(history.len(), 1);
}

#[tokio::test]
async fn callback_rejects_metadata_only_session_routing() {
    let fixture = Fixture::new(None).await;

    let response = fixture
        .router
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-indexed",
                "agent_id": "agent-1",
                "session_id": "",
                "event_type": "ProgressUpdate",
                "event_id": "event-indexed",
                "timestamp": "2026-05-10T09:00:00Z",
                "metadata": {"session_id": "http-conversation"},
                "data": {"summary": "found by index"}
            }),
        ))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    assert!(fixture.events.events.lock().unwrap().is_empty());
}

#[tokio::test]
async fn callback_returns_not_found_for_unknown_session_id() {
    let fixture = Fixture::new(None).await;

    let response = fixture
        .router
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-unknown",
                "agent_id": "agent-1",
                "session_id": "http-missing",
                "event_type": "ProgressUpdate",
                "event_id": "event-unknown",
                "timestamp": "2026-05-10T09:00:00Z",
                "metadata": {"session_id": "http-conversation"},
                "data": {"summary": "missing"}
            }),
        ))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::NOT_FOUND);
    assert!(fixture.events.events.lock().unwrap().is_empty());
}

#[tokio::test]
async fn callback_delivers_to_existing_slack_session() {
    let deliverer = Arc::new(RecordingCallbackDeliverer::default());
    let callback_deliverer: Arc<dyn api::CloudAgentCallbackDeliverer> = deliverer.clone();
    let fixture = Fixture::new_with_deliverer(None, Some(callback_deliverer)).await;
    fixture
        .sessions
        .insert(slack_session("C123-1770000000.000100"));

    let response = fixture
        .router
        .oneshot(callback_request(
            Some("secret"),
            json!({
                "task_id": "task-slack",
                "agent_id": "agent-1",
                "session_id": "C123-1770000000.000100",
                "event_type": "todo_updated",
                "event_id": "event-slack",
                "timestamp": "2026-05-10T09:00:00Z",
                "metadata": {"session_id": "http-conversation"},
                "data": {"summary": "todo changed"}
            }),
        ))
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::ACCEPTED);
    let deliveries = deliverer.deliveries.lock().unwrap();
    assert_eq!(deliveries.len(), 1);
    assert_eq!(deliveries[0].0, "C123-1770000000.000100");
    assert_eq!(deliveries[0].1["event_id"], "event-slack");
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
                "session_id": "http-conversation",
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
                "session_id": "http-conversation",
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
    http_stream_id: String,
}

impl Fixture {
    async fn new(cloud_task_index: Option<Arc<CloudTaskIndex>>) -> Self {
        Self::new_with_deliverer(cloud_task_index, None).await
    }

    async fn new_with_deliverer(
        cloud_task_index: Option<Arc<CloudTaskIndex>>,
        callback_deliverer: Option<Arc<dyn api::CloudAgentCallbackDeliverer>>,
    ) -> Self {
        let config = ConfigStore::new(test_definition());
        let config_repo = Arc::new(MemoryConfigRepo::default());
        let sessions = Arc::new(MemorySessionRepo::default());
        sessions.insert(session("http-conversation"));
        let events = Arc::new(MemoryEventRepo::default());
        let (tx, _rx) = mpsc::channel(1);
        let broker = Arc::new(api::HttpStreamBroker::new());
        let http_stream_id = broker.create_stream().await;
        broker
            .register_session("http-conversation", &http_stream_id)
            .await;
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
            None,
            cloud_task_index,
            callback_deliverer,
        );
        Self {
            router: api::build_router(state),
            sessions,
            events,
            broker,
            http_stream_id,
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

fn slack_session(id: &str) -> Session {
    let now = Utc::now();
    Session {
        id: SessionId::from(id),
        channel: "C123".to_string(),
        thread_ts: "1770000000.000100".to_string(),
        agent_session_id: id.to_string(),
        status: SessionStatus::Active,
        created_at: now,
        last_activity_at: now,
    }
}

#[derive(Default)]
struct RecordingCallbackDeliverer {
    deliveries: Mutex<Vec<(String, Value)>>,
}

#[async_trait]
impl api::CloudAgentCallbackDeliverer for RecordingCallbackDeliverer {
    async fn deliver_cloud_agent_callback(
        &self,
        session_id: &SessionId,
        payload: Value,
    ) -> anyhow::Result<()> {
        self.deliveries
            .lock()
            .unwrap()
            .push((session_id.as_str().to_string(), payload));
        Ok(())
    }
}

fn test_definition() -> AgentDefinition {
    AgentDefinition {
        agent: AgentMeta {
            name: "Test".to_string(),
            description: String::new(),
            system_prompt: "test".to_string(),
        },
        prompt_fragments: Default::default(),
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
struct RecordingOutboundReloader {
    reloads: Mutex<Vec<Vec<String>>>,
}

#[async_trait]
impl api::OutboundConfigReloader for RecordingOutboundReloader {
    async fn reload_outbound_channels(&self, specs: &[OutboundChannelSpec]) -> anyhow::Result<()> {
        self.reloads
            .lock()
            .unwrap()
            .push(specs.iter().map(|spec| spec.name.clone()).collect());
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
        _filter: storage::SessionListFilter,
        _limit: u32,
    ) -> storage::Result<Vec<Session>> {
        Ok(self.sessions.lock().unwrap().values().cloned().collect())
    }
}

#[derive(Default)]
struct MemoryEventRepo {
    events: Mutex<Vec<SessionEvent>>,
    idempotency_keys: Mutex<HashMap<String, i64>>,
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

    async fn append_idempotent(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: Value,
        idempotency_key: &str,
    ) -> storage::Result<Option<i64>> {
        if self
            .idempotency_keys
            .lock()
            .unwrap()
            .contains_key(idempotency_key)
        {
            return Ok(None);
        }
        let id = self.append(session_id, kind, payload).await?;
        self.idempotency_keys
            .lock()
            .unwrap()
            .insert(idempotency_key.to_string(), id);
        Ok(Some(id))
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
