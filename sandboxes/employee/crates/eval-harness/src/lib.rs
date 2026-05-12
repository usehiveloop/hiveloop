use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{
    AgentDefinition, AgentMeta, ConfigStore, EventKind, ModelConfig, Session, SessionEvent,
    SessionId, SessionStatus,
};
use observability::{ModelUsage, ObservabilityEvent, TraceSummary};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo, StorageError};
use tokio::sync::{mpsc, oneshot};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EvalCase {
    pub id: String,
    pub input: String,
    #[serde(default)]
    pub expected_substrings: Vec<String>,
    #[serde(default)]
    pub min_tool_calls: usize,
    #[serde(default)]
    pub max_errors: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EvalCaseResult {
    pub case_id: String,
    pub passed: bool,
    pub failures: Vec<String>,
    pub trace_id: String,
    pub session_id: String,
    pub turn_id: String,
    pub summary: TraceSummary,
    pub events: Vec<ObservabilityEvent>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EvalRunSummary {
    pub total: usize,
    pub passed: usize,
    pub failed: usize,
    pub cases: Vec<EvalCaseResult>,
}

#[derive(Clone)]
pub struct HttpEvalRunner {
    client: reqwest::Client,
    base_url: String,
    bearer_token: String,
    poll_interval: Duration,
    poll_attempts: usize,
}

impl HttpEvalRunner {
    pub fn new(base_url: impl Into<String>, bearer_token: impl Into<String>) -> Self {
        Self {
            client: reqwest::Client::new(),
            base_url: base_url.into().trim_end_matches('/').to_string(),
            bearer_token: bearer_token.into(),
            poll_interval: Duration::from_millis(10),
            poll_attempts: 100,
        }
    }

    pub async fn run_cases(&self, cases: &[EvalCase]) -> Result<EvalRunSummary> {
        let mut results = Vec::with_capacity(cases.len());
        for case in cases {
            results.push(self.run_case(case).await?);
        }
        let passed = results.iter().filter(|result| result.passed).count();
        Ok(EvalRunSummary {
            total: results.len(),
            passed,
            failed: results.len() - passed,
            cases: results,
        })
    }

    pub async fn run_case(&self, case: &EvalCase) -> Result<EvalCaseResult> {
        let response: HttpMessageResponse = self
            .client
            .post(format!("{}/gateway/http/messages", self.base_url))
            .bearer_auth(&self.bearer_token)
            .json(&json!({
                "text": case.input,
                "conversation_id": format!("eval-{}", case.id),
                "user": "eval-harness",
                "raw": {
                    "eval_case_id": case.id,
                }
            }))
            .send()
            .await
            .context("post eval message")?
            .error_for_status()
            .context("post eval message status")?
            .json()
            .await
            .context("decode eval message response")?;

        let summary = self.wait_for_summary(&response.trace_id).await?;
        let events: Vec<ObservabilityEvent> = self
            .client
            .get(format!(
                "{}/observability/traces/{}/events",
                self.base_url, response.trace_id
            ))
            .bearer_auth(&self.bearer_token)
            .send()
            .await
            .context("fetch trace events")?
            .error_for_status()
            .context("fetch trace events status")?
            .json()
            .await
            .context("decode trace events")?;

        let failures = assert_case(case, &summary);
        Ok(EvalCaseResult {
            case_id: case.id.clone(),
            passed: failures.is_empty(),
            failures,
            trace_id: response.trace_id,
            session_id: response.session_id,
            turn_id: response.turn_id,
            summary,
            events,
        })
    }

    async fn wait_for_summary(&self, trace_id: &str) -> Result<TraceSummary> {
        let mut last_event_count = 0;
        for _ in 0..self.poll_attempts {
            let summary: TraceSummary = self
                .client
                .get(format!(
                    "{}/observability/traces/{trace_id}/summary",
                    self.base_url
                ))
                .bearer_auth(&self.bearer_token)
                .send()
                .await
                .context("fetch trace summary")?
                .error_for_status()
                .context("fetch trace summary status")?
                .json()
                .await
                .context("decode trace summary")?;
            if summary.is_complete {
                return Ok(summary);
            }
            last_event_count = summary.event_count;
            tokio::time::sleep(self.poll_interval).await;
        }
        Err(anyhow!(
            "trace `{trace_id}` did not complete after {} poll attempts (last event count: {last_event_count})",
            self.poll_attempts
        ))
    }
}

fn assert_case(case: &EvalCase, summary: &TraceSummary) -> Vec<String> {
    let mut failures = Vec::new();
    let final_text = summary.final_text.as_deref().unwrap_or_default();
    for expected in &case.expected_substrings {
        if !final_text.contains(expected) {
            failures.push(format!("final text did not contain `{expected}`"));
        }
    }
    if summary.tool_call_count < case.min_tool_calls {
        failures.push(format!(
            "expected at least {} tool call(s), got {}",
            case.min_tool_calls, summary.tool_call_count
        ));
    }
    if summary.error_count > case.max_errors {
        failures.push(format!(
            "expected at most {} error(s), got {}",
            case.max_errors, summary.error_count
        ));
    }
    failures
}

#[derive(Debug, Clone)]
pub struct FakeProviderScript {
    pub final_text_template: String,
    pub model_usage: ModelUsage,
    pub tool_calls: Vec<FakeToolCall>,
}

impl Default for FakeProviderScript {
    fn default() -> Self {
        Self {
            final_text_template: "fake response: {input}".to_string(),
            model_usage: ModelUsage {
                provider: Some("fake".to_string()),
                model: Some("fake-model".to_string()),
                prompt_tokens: 8,
                completion_tokens: 5,
                total_tokens: 13,
                cached_tokens: 0,
                cache_write_tokens: 0,
                cost: Some(0.0),
            },
            tool_calls: Vec::new(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct FakeToolCall {
    pub id: String,
    pub tool_name: String,
    pub args: Value,
    pub result: Value,
}

pub struct FakeGatewayServer {
    pub base_url: String,
    pub bearer_token: String,
    shutdown: Option<oneshot::Sender<()>>,
    server_handle: tokio::task::JoinHandle<()>,
    provider_handle: tokio::task::JoinHandle<()>,
}

impl FakeGatewayServer {
    pub async fn spawn(script: FakeProviderScript) -> Result<Self> {
        let bearer_token = "eval-token".to_string();
        let broker = Arc::new(api::HttpStreamBroker::new());
        let (inbound_sink, inbound_rx) = mpsc::channel(32);
        let state = api::ApiState::new(
            ConfigStore::new(fake_agent_definition()),
            Arc::new(NoopConfigRepo),
            Arc::new(NoopSessionRepo),
            Arc::new(NoopEventRepo),
            bearer_token.clone(),
            Arc::new(SkillWriter::new(
                std::env::temp_dir().join("eval-harness-skills"),
            )),
            Some(api::HttpGatewayState {
                inbound_sink,
                broker: broker.clone(),
            }),
            None,
            None,
            None,
        );
        state.mark_config_loaded();
        state.mark_gateway_ready();

        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .context("bind fake eval gateway")?;
        let addr = listener.local_addr().context("read fake gateway addr")?;
        let router = api::build_router(state);
        let (shutdown_tx, shutdown_rx) = oneshot::channel();
        let server_handle = tokio::spawn(async move {
            let _ = axum::serve(listener, router)
                .with_graceful_shutdown(async move {
                    let _ = shutdown_rx.await;
                })
                .await;
        });
        let provider_handle = tokio::spawn(run_fake_provider(inbound_rx, broker, script));

        Ok(Self {
            base_url: format!("http://{addr}"),
            bearer_token,
            shutdown: Some(shutdown_tx),
            server_handle,
            provider_handle,
        })
    }

    pub fn runner(&self) -> HttpEvalRunner {
        HttpEvalRunner::new(self.base_url.clone(), self.bearer_token.clone())
    }

    pub async fn shutdown(mut self) {
        if let Some(shutdown) = self.shutdown.take() {
            let _ = shutdown.send(());
        }
        self.provider_handle.abort();
        self.server_handle.abort();
    }
}

impl Drop for FakeGatewayServer {
    fn drop(&mut self) {
        if let Some(shutdown) = self.shutdown.take() {
            let _ = shutdown.send(());
        }
        self.provider_handle.abort();
    }
}

async fn run_fake_provider(
    mut inbound_rx: mpsc::Receiver<domain::InboundEvent>,
    broker: Arc<api::HttpStreamBroker>,
    script: FakeProviderScript,
) {
    while let Some(inbound) = inbound_rx.recv().await {
        let Some(stream_id) = inbound
            .raw
            .get("http_stream_id")
            .and_then(serde_json::Value::as_str)
        else {
            continue;
        };
        broker
            .publish(
                stream_id,
                "thinking",
                json!({
                    "session_id": inbound.session_id.as_str(),
                    "text": "fake provider planning",
                }),
            )
            .await;

        for call in &script.tool_calls {
            broker
                .publish(
                    stream_id,
                    "tool_call",
                    json!({
                        "session_id": inbound.session_id.as_str(),
                        "id": call.id.clone(),
                        "tool": call.tool_name.clone(),
                        "args": call.args.clone(),
                    }),
                )
                .await;
            broker
                .publish(
                    stream_id,
                    "tool_result",
                    json!({
                        "session_id": inbound.session_id.as_str(),
                        "id": call.id.clone(),
                        "tool": call.tool_name.clone(),
                        "result": call.result.clone(),
                    }),
                )
                .await;
        }

        broker
            .publish(
                stream_id,
                "model_usage",
                json!({
                    "session_id": inbound.session_id.as_str(),
                    "provider": script.model_usage.provider.clone(),
                    "model": script.model_usage.model.clone(),
                    "usage": {
                        "prompt_tokens": script.model_usage.prompt_tokens,
                        "completion_tokens": script.model_usage.completion_tokens,
                        "total_tokens": script.model_usage.total_tokens,
                        "cached_tokens": script.model_usage.cached_tokens,
                        "cache_write_tokens": script.model_usage.cache_write_tokens,
                        "cost": script.model_usage.cost,
                    }
                }),
            )
            .await;

        let final_text = script.final_text_template.replace("{input}", &inbound.text);
        for token in final_text.split_inclusive(' ') {
            broker
                .publish(
                    stream_id,
                    "token",
                    json!({
                        "session_id": inbound.session_id.as_str(),
                        "text": token,
                    }),
                )
                .await;
        }
        broker
            .publish(
                stream_id,
                "final",
                json!({
                    "session_id": inbound.session_id.as_str(),
                    "text": final_text,
                }),
            )
            .await;
        broker
            .publish(
                stream_id,
                "done",
                json!({
                    "session_id": inbound.session_id.as_str(),
                }),
            )
            .await;
    }
}

fn fake_agent_definition() -> AgentDefinition {
    AgentDefinition {
        agent: AgentMeta {
            name: "Eval Fake".to_string(),
            description: "Deterministic eval fake".to_string(),
            system_prompt: "Return deterministic fake responses.".to_string(),
        },
        prompt_fragments: Default::default(),
        model: ModelConfig::OpenaiCompatible {
            base_url: "http://127.0.0.1/fake".to_string(),
            model_id: "fake-model".to_string(),
            api_key_env: "FAKE_API_KEY".to_string(),
            temperature: Some(0.0),
            max_output_tokens: Some(128),
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
        subagents: Vec::new(),
        slack: Default::default(),
        outbound_channels: Vec::new(),
    }
}

#[derive(Debug, Deserialize)]
struct HttpMessageResponse {
    session_id: String,
    trace_id: String,
    turn_id: String,
}

struct NoopConfigRepo;

#[async_trait]
impl ConfigRepo for NoopConfigRepo {
    async fn load(&self) -> storage::Result<Option<AgentDefinition>> {
        Ok(Some(fake_agent_definition()))
    }

    async fn upsert(&self, _def: &AgentDefinition) -> storage::Result<()> {
        Ok(())
    }
}

struct NoopSessionRepo;

#[async_trait]
impl SessionRepo for NoopSessionRepo {
    async fn get(&self, _id: &SessionId) -> storage::Result<Option<Session>> {
        Ok(None)
    }

    async fn create(&self, _session: &Session) -> storage::Result<()> {
        Ok(())
    }

    async fn touch(&self, _id: &SessionId, _at: DateTime<Utc>) -> storage::Result<()> {
        Ok(())
    }

    async fn set_status(&self, _id: &SessionId, _status: SessionStatus) -> storage::Result<()> {
        Ok(())
    }

    async fn list(
        &self,
        _cursor: Option<DateTime<Utc>>,
        _status: Option<SessionStatus>,
        _limit: u32,
    ) -> storage::Result<Vec<Session>> {
        Ok(Vec::new())
    }
}

struct NoopEventRepo;

#[async_trait]
impl EventRepo for NoopEventRepo {
    async fn append(
        &self,
        _session_id: &SessionId,
        _kind: EventKind,
        _payload: Value,
    ) -> storage::Result<i64> {
        Err(StorageError::Other(anyhow!(
            "NoopEventRepo does not append"
        )))
    }

    async fn append_idempotent(
        &self,
        _session_id: &SessionId,
        _kind: EventKind,
        _payload: Value,
        _idempotency_key: &str,
    ) -> storage::Result<Option<i64>> {
        Err(StorageError::Other(anyhow!(
            "NoopEventRepo does not append"
        )))
    }

    async fn list_recent(
        &self,
        _session_id: &SessionId,
        _limit: u32,
    ) -> storage::Result<Vec<SessionEvent>> {
        Ok(Vec::new())
    }

    async fn list_chronological(
        &self,
        _session_id: &SessionId,
        _limit: u32,
    ) -> storage::Result<Vec<SessionEvent>> {
        Ok(Vec::new())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn deterministic_fake_gateway_eval_produces_summary_and_events() {
        let server = FakeGatewayServer::spawn(FakeProviderScript {
            final_text_template: "answer for {input}".to_string(),
            tool_calls: vec![FakeToolCall {
                id: "call-1".to_string(),
                tool_name: "lookup".to_string(),
                args: json!({"query": "fixed"}),
                result: json!({"value": "fixed-result"}),
            }],
            ..FakeProviderScript::default()
        })
        .await
        .expect("spawn fake gateway");

        let runner = server.runner();
        let summary = runner
            .run_cases(&[EvalCase {
                id: "case-1".to_string(),
                input: "hello eval".to_string(),
                expected_substrings: vec!["answer for hello eval".to_string()],
                min_tool_calls: 1,
                max_errors: 0,
            }])
            .await
            .expect("run eval");

        assert_eq!(summary.total, 1);
        assert_eq!(summary.passed, 1);
        let result = &summary.cases[0];
        assert!(result.passed, "{:?}", result.failures);
        assert!(result.trace_id.starts_with("trace-http-stream-"));
        assert_eq!(result.summary.tool_call_count, 1);
        assert_eq!(result.summary.model_usage.total_tokens, 13);
        assert!(result.events.iter().all(|event| !event.trace_id.is_empty()
            && !event.session_id.is_empty()
            && !event.turn_id.is_empty()));

        server.shutdown().await;
    }
}
