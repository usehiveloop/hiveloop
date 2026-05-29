use std::collections::HashMap;
use std::pin::Pin;
use std::time::Duration;

use async_stream::stream;
use domain::{ModelConfig, ReasoningEffort};
use futures::{Stream, StreamExt};
use reqwest::{Client, Response};
use safety::json_repair::JsonRepair;
use serde_json::Value;

use crate::primitives::{
    CacheControlPolicy, ModelRequest, ModelStreamEvent, ProviderUsage, ToolCall,
};
use crate::request_builder::build_openai_compatible_request;
use crate::{AgentError, Result};

pub type ModelEventStream = Pin<Box<dyn Stream<Item = Result<ModelStreamEvent>> + Send>>;

#[derive(Debug, Clone)]
pub struct ChatModelClient {
    http: Client,
    endpoints: Vec<ModelEndpoint>,
    retry_policy: ModelRetryPolicy,
    json_repair: JsonRepair,
}

impl ChatModelClient {
    pub fn new(base_url: impl Into<String>, api_key: impl Into<String>) -> Self {
        Self {
            http: Client::new(),
            endpoints: vec![ModelEndpoint {
                base_url: base_url.into().trim_end_matches('/').to_string(),
                api_key: api_key.into(),
                model_id: None,
                extra_headers: HashMap::new(),
                cache_policy: CacheControlPolicy::Disabled,
            }],
            retry_policy: ModelRetryPolicy::default(),
            json_repair: JsonRepair::new(),
        }
    }

    pub fn from_model_config(
        model: &ModelConfig,
        runtime_env: &HashMap<String, String>,
    ) -> Result<ModelClientConfig> {
        let mut endpoints = Vec::new();
        collect_model_endpoints(model, &mut endpoints, runtime_env)?;
        let Some(primary) = endpoints.first() else {
            return Err(AgentError::Model(
                "model config has no endpoints".to_string(),
            ));
        };
        let model_id = primary.model_id.clone().unwrap_or_default();
        let cache_policy = primary.cache_policy;
        Ok(ModelClientConfig {
            client: Self {
                http: Client::new(),
                endpoints,
                retry_policy: ModelRetryPolicy::default(),
                json_repair: JsonRepair::new(),
            },
            model_id,
            cache_policy,
            reasoning_effort: primary_reasoning_effort(model),
            temperature: primary_temperature(model),
            max_output_tokens: primary_max_output_tokens(model),
        })
    }

    pub fn with_retry_policy(mut self, retry_policy: ModelRetryPolicy) -> Self {
        self.retry_policy = retry_policy.normalized();
        self
    }

    pub async fn stream(&self, request: ModelRequest) -> Result<ModelEventStream> {
        let mut last_failure = None;

        for (endpoint_index, endpoint) in self.endpoints.iter().enumerate() {
            for attempt in 1..=self.retry_policy.max_attempts {
                match self.send_once(endpoint, &request).await {
                    Ok(response) => return Ok(stream_response(response, &self.json_repair)),
                    Err(failure) => {
                        let should_retry = failure.class.is_retryable()
                            && attempt < self.retry_policy.max_attempts;
                        let fallback_available = endpoint_index + 1 < self.endpoints.len();
                        tracing::warn!(
                            error_class = failure.class.as_str(),
                            status = failure.status,
                            attempt,
                            max_attempts = self.retry_policy.max_attempts,
                            retrying = should_retry,
                            fallback_available,
                            endpoint_index,
                            model = endpoint
                                .model_id
                                .as_deref()
                                .unwrap_or(request.model.as_str()),
                            error = %failure.message,
                            "model request failed"
                        );

                        if should_retry {
                            self.retry_policy.sleep_before_retry(attempt).await;
                            continue;
                        }

                        let should_fallback = fallback_available && failure.class.allows_fallback();
                        last_failure = Some(failure);
                        if should_fallback {
                            break;
                        }

                        return Err(last_failure
                            .expect("last failure is set before return")
                            .into_agent_error());
                    }
                }
            }
        }

        Err(last_failure
            .map(ModelRequestFailure::into_agent_error)
            .unwrap_or_else(|| AgentError::Model("model request failed".to_string())))
    }

    async fn send_once(
        &self,
        endpoint: &ModelEndpoint,
        request: &ModelRequest,
    ) -> std::result::Result<Response, ModelRequestFailure> {
        let mut endpoint_request = request.clone();
        if let Some(model_id) = &endpoint.model_id {
            endpoint_request.model = model_id.clone();
        }
        let body = build_openai_compatible_request(&endpoint_request);
        tracing::debug!(
            bytes = body.to_string().len(),
            tools = request.tools.len(),
            messages = request.messages.len(),
            model = endpoint_request.model,
            "sending model request"
        );
        let mut builder = self
            .http
            .post(format!("{}/chat/completions", endpoint.base_url))
            .bearer_auth(&endpoint.api_key)
            .json(&body);
        for (name, value) in &endpoint.extra_headers {
            builder = builder.header(name, value);
        }
        let response = builder
            .send()
            .await
            .map_err(ModelRequestFailure::from_transport_error)?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().await.unwrap_or_default();
            return Err(ModelRequestFailure::from_http_error(status.as_u16(), text));
        }

        Ok(response)
    }
}

#[derive(Debug, Clone)]
pub struct ModelClientConfig {
    pub client: ChatModelClient,
    pub model_id: String,
    pub cache_policy: CacheControlPolicy,
    pub reasoning_effort: Option<String>,
    pub temperature: Option<f32>,
    pub max_output_tokens: Option<u32>,
}

#[derive(Debug, Clone)]
struct ModelEndpoint {
    base_url: String,
    api_key: String,
    model_id: Option<String>,
    extra_headers: HashMap<String, String>,
    cache_policy: CacheControlPolicy,
}

#[derive(Debug, Clone)]
pub struct ModelRetryPolicy {
    pub max_attempts: usize,
    pub initial_backoff: Duration,
    pub max_backoff: Duration,
}

impl Default for ModelRetryPolicy {
    fn default() -> Self {
        Self {
            max_attempts: 3,
            initial_backoff: Duration::from_millis(200),
            max_backoff: Duration::from_secs(2),
        }
    }
}

impl ModelRetryPolicy {
    pub fn no_delay(max_attempts: usize) -> Self {
        Self {
            max_attempts: max_attempts.max(1),
            initial_backoff: Duration::ZERO,
            max_backoff: Duration::ZERO,
        }
    }

    fn normalized(mut self) -> Self {
        self.max_attempts = self.max_attempts.max(1);
        self
    }

    async fn sleep_before_retry(&self, attempt: usize) {
        let multiplier = 1_u32.checked_shl((attempt - 1).min(31) as u32).unwrap_or(1);
        let delay = self
            .initial_backoff
            .saturating_mul(multiplier)
            .min(self.max_backoff);
        if !delay.is_zero() {
            tokio::time::sleep(delay).await;
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ModelErrorClass {
    Timeout,
    Transport,
    RateLimited,
    Server,
    Overloaded,
    Auth,
    Billing,
    ContextLength,
    InvalidRequest,
    Unknown,
}

impl ModelErrorClass {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Timeout => "timeout",
            Self::Transport => "transport",
            Self::RateLimited => "rate_limited",
            Self::Server => "server",
            Self::Overloaded => "overloaded",
            Self::Auth => "auth",
            Self::Billing => "billing",
            Self::ContextLength => "context_length",
            Self::InvalidRequest => "invalid_request",
            Self::Unknown => "unknown",
        }
    }

    fn is_retryable(self) -> bool {
        matches!(
            self,
            Self::Timeout | Self::Transport | Self::RateLimited | Self::Server | Self::Overloaded
        )
    }

    fn allows_fallback(self) -> bool {
        !matches!(self, Self::ContextLength | Self::InvalidRequest)
    }
}

#[derive(Debug, Clone)]
struct ModelRequestFailure {
    class: ModelErrorClass,
    status: Option<u16>,
    message: String,
}

impl ModelRequestFailure {
    fn from_transport_error(error: reqwest::Error) -> Self {
        let class = if error.is_timeout() {
            ModelErrorClass::Timeout
        } else {
            ModelErrorClass::Transport
        };
        Self {
            class,
            status: None,
            message: error.to_string(),
        }
    }

    fn from_http_error(status: u16, body: String) -> Self {
        let message = extract_model_error_message(&body);
        Self {
            class: classify_http_error(status, &message, &body),
            status: Some(status),
            message,
        }
    }

    fn into_agent_error(self) -> AgentError {
        let status = self
            .status
            .map(|status| format!("HTTP {status}: "))
            .unwrap_or_default();
        AgentError::Model(format!(
            "{}{} ({})",
            status,
            self.message,
            self.class.as_str()
        ))
    }
}

fn stream_response(response: Response, json_repair: &JsonRepair) -> ModelEventStream {
    let bytes = response.bytes_stream();
    let json_repair = json_repair.clone();
    Box::pin(stream! {
        let mut buffer = String::new();
        let mut tool_accumulator = ToolCallAccumulator::default();
        futures::pin_mut!(bytes);
        while let Some(chunk) = bytes.next().await {
            let chunk = match chunk {
                Ok(chunk) => chunk,
                Err(error) => {
                    let failure = ModelRequestFailure::from_transport_error(error);
                    yield Err(failure.into_agent_error());
                    return;
                }
            };
            buffer.push_str(&String::from_utf8_lossy(&chunk));
            while let Some(event) = take_sse_event(&mut buffer) {
                let data = event.trim_start_matches("data:").trim();
                if data.is_empty() || data.starts_with(':') {
                    continue;
                }
                if data == "[DONE]" {
                    let calls = tool_accumulator.finish(&json_repair);
                    if !calls.is_empty() {
                        yield Ok(ModelStreamEvent::ToolCalls(calls));
                    }
                    yield Ok(ModelStreamEvent::Done);
                    return;
                }
                let Ok(value) = serde_json::from_str::<Value>(data) else {
                    continue;
                };
                if let Some(usage) = parse_usage(&value) {
                    yield Ok(ModelStreamEvent::Usage(usage));
                }
                if let Some(choices) = value.get("choices").and_then(|v| v.as_array()) {
                    for choice in choices {
                        let Some(delta) = choice.get("delta") else { continue };
                        if let Some(text) = delta.get("content").and_then(|v| v.as_str()) {
                            if !text.is_empty() {
                                yield Ok(ModelStreamEvent::TextDelta(text.to_string()));
                            }
                        }
                        if let Some(text) = parse_thinking_delta(delta) {
                            if !text.is_empty() {
                                yield Ok(ModelStreamEvent::ThinkingDelta(text));
                            }
                        }
                        if let Some(tool_calls) = delta.get("tool_calls").and_then(|v| v.as_array()) {
                            tool_accumulator.apply_delta(tool_calls);
                        }
                    }
                }
            }
        }
    })
}

fn collect_model_endpoints(
    model: &ModelConfig,
    endpoints: &mut Vec<ModelEndpoint>,
    runtime_env: &HashMap<String, String>,
) -> Result<()> {
    match model {
        ModelConfig::OpenaiCompatible {
            base_url,
            model_id,
            api_key_env,
            extra_headers,
            fallback,
            ..
        } => {
            let api_key = runtime_env
                .get(api_key_env)
                .cloned()
                .ok_or_else(|| AgentError::Model(format!("env var `{api_key_env}` not set")))?;
            endpoints.push(ModelEndpoint {
                base_url: base_url.trim_end_matches('/').to_string(),
                api_key,
                model_id: Some(model_id.clone()),
                extra_headers: extra_headers.clone(),
                cache_policy: cache_policy_for_values(base_url, api_key_env),
            });
            if let Some(fallback) = fallback {
                collect_model_endpoints(fallback, endpoints, runtime_env)?;
            }
            Ok(())
        }
    }
}

fn primary_temperature(model: &ModelConfig) -> Option<f32> {
    match model {
        ModelConfig::OpenaiCompatible { temperature, .. } => *temperature,
    }
}

fn primary_max_output_tokens(model: &ModelConfig) -> Option<u32> {
    match model {
        ModelConfig::OpenaiCompatible {
            max_output_tokens, ..
        } => *max_output_tokens,
    }
}

fn primary_reasoning_effort(model: &ModelConfig) -> Option<String> {
    match model {
        ModelConfig::OpenaiCompatible {
            reasoning_effort, ..
        } => reasoning_effort.map(reasoning_effort_to_string),
    }
}

fn reasoning_effort_to_string(effort: ReasoningEffort) -> String {
    match effort {
        ReasoningEffort::Low => "low".to_string(),
        ReasoningEffort::Medium => "medium".to_string(),
        ReasoningEffort::High => "high".to_string(),
    }
}

fn cache_policy_for_values(base_url: &str, _api_key_env: &str) -> CacheControlPolicy {
    if base_url.contains("openrouter") || base_url.contains("127.0.0.1") {
        CacheControlPolicy::OpenRouterGeminiEphemeral
    } else {
        CacheControlPolicy::Disabled
    }
}

fn classify_http_error(status: u16, message: &str, body: &str) -> ModelErrorClass {
    let haystack = format!("{message}\n{body}").to_ascii_lowercase();
    if status == 408 || haystack.contains("timeout") || haystack.contains("timed out") {
        return ModelErrorClass::Timeout;
    }
    if status == 429 || haystack.contains("rate limit") || haystack.contains("too many requests") {
        return ModelErrorClass::RateLimited;
    }
    if status == 529
        || haystack.contains("overloaded")
        || haystack.contains("capacity")
        || haystack.contains("temporarily unavailable")
    {
        return ModelErrorClass::Overloaded;
    }
    if status == 401 || status == 403 || haystack.contains("api key") {
        return ModelErrorClass::Auth;
    }
    if status == 402
        || haystack.contains("billing")
        || haystack.contains("insufficient credit")
        || haystack.contains("insufficient balance")
        || haystack.contains("payment required")
    {
        return ModelErrorClass::Billing;
    }
    if status == 413
        || haystack.contains("context length")
        || haystack.contains("context window")
        || haystack.contains("maximum context")
        || haystack.contains("too many tokens")
        || haystack.contains("token limit")
    {
        return ModelErrorClass::ContextLength;
    }
    if (500..=599).contains(&status) {
        return ModelErrorClass::Server;
    }
    if matches!(status, 400 | 404 | 422) {
        return ModelErrorClass::InvalidRequest;
    }
    ModelErrorClass::Unknown
}

fn extract_model_error_message(body: &str) -> String {
    let Ok(value) = serde_json::from_str::<Value>(body) else {
        return body.to_string();
    };
    value
        .get("error")
        .and_then(|error| {
            error
                .get("message")
                .or_else(|| error.get("code"))
                .and_then(Value::as_str)
        })
        .or_else(|| value.get("message").and_then(Value::as_str))
        .map(ToString::to_string)
        .unwrap_or_else(|| body.to_string())
}

fn take_sse_event(buffer: &mut String) -> Option<String> {
    let idx = buffer.find("\n\n")?;
    let event = buffer[..idx].to_string();
    buffer.replace_range(..idx + 2, "");
    Some(event)
}

fn parse_thinking_delta(delta: &Value) -> Option<String> {
    for key in ["reasoning", "reasoning_content", "thinking"] {
        if let Some(text) = delta.get(key).and_then(Value::as_str) {
            return Some(text.to_string());
        }
    }
    delta
        .get("reasoning")
        .or_else(|| delta.get("thinking"))
        .and_then(|value| value.get("content"))
        .and_then(Value::as_str)
        .map(ToString::to_string)
}

fn parse_usage(value: &Value) -> Option<ProviderUsage> {
    let usage = value.get("usage")?;
    let prompt_details = usage.get("prompt_tokens_details").unwrap_or(&Value::Null);
    let completion_details = usage
        .get("completion_tokens_details")
        .unwrap_or(&Value::Null);
    Some(ProviderUsage {
        prompt_tokens: usage
            .get("prompt_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        completion_tokens: usage
            .get("completion_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        total_tokens: usage
            .get("total_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cached_tokens: prompt_details
            .get("cached_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cache_write_tokens: prompt_details
            .get("cache_write_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        reasoning_tokens: completion_details
            .get("reasoning_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cost: usage.get("cost").and_then(|v| v.as_f64()),
        raw: Some(usage.clone()),
    })
}

#[derive(Default)]
struct ToolCallAccumulator {
    calls: Vec<PartialToolCall>,
}

impl ToolCallAccumulator {
    fn apply_delta(&mut self, deltas: &[Value]) {
        for delta in deltas {
            let index = delta.get("index").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
            while self.calls.len() <= index {
                self.calls.push(PartialToolCall::default());
            }
            let call = &mut self.calls[index];
            if let Some(id) = delta.get("id").and_then(|v| v.as_str()) {
                call.id = id.to_string();
            }
            if let Some(function) = delta.get("function") {
                if let Some(name) = function.get("name").and_then(|v| v.as_str()) {
                    call.name.push_str(name);
                }
                if let Some(arguments) = function.get("arguments").and_then(|v| v.as_str()) {
                    call.arguments.push_str(arguments);
                }
            }
        }
    }

    fn finish(self, repair: &JsonRepair) -> Vec<ToolCall> {
        self.calls
            .into_iter()
            .filter(|call| !call.name.is_empty())
            .map(|call| {
                let (repaired_args, was_repaired) = repair.repair(&call.arguments);
                if was_repaired {
                    tracing::debug!(
                        tool = call.name,
                        raw = call.arguments,
                        "repaired malformed JSON tool arguments"
                    );
                }
                ToolCall {
                    id: if call.id.is_empty() {
                        format!("tool_{}", call.name)
                    } else {
                        call.id
                    },
                    name: call.name,
                    arguments: repaired_args,
                }
            })
            .collect()
    }
}

#[derive(Default)]
struct PartialToolCall {
    id: String,
    name: String,
    arguments: String,
}

#[cfg(test)]
mod tests {
    use std::collections::{HashMap, VecDeque};
    use std::net::SocketAddr;
    use std::sync::Arc;
    use std::time::UNIX_EPOCH;

    use axum::extract::State;
    use axum::http::{header, StatusCode};
    use axum::response::{IntoResponse, Response};
    use axum::routing::post;
    use axum::{Json, Router};
    use domain::ModelConfig;
    use futures::StreamExt;
    use serde_json::json;
    use tokio::net::TcpListener;
    use tokio::sync::Mutex;

    use crate::primitives::{AgentMessage, CacheControlPolicy, ModelRequest, ModelStreamEvent};

    use super::{
        classify_http_error, parse_thinking_delta, parse_usage, ChatModelClient, ModelErrorClass,
        ModelRetryPolicy,
    };

    #[test]
    fn parses_common_thinking_delta_fields() {
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning": "thinking A"})),
            Some("thinking A".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning_content": "thinking B"})),
            Some("thinking B".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"thinking": "thinking C"})),
            Some("thinking C".to_string())
        );
        assert_eq!(
            parse_thinking_delta(&json!({"reasoning": {"content": "thinking D"}})),
            Some("thinking D".to_string())
        );
    }

    #[test]
    fn ignores_missing_thinking_delta() {
        assert_eq!(parse_thinking_delta(&json!({"content": "visible"})), None);
    }

    #[test]
    fn parses_reasoning_tokens_from_usage_details() {
        let usage = parse_usage(&json!({
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 7,
                "total_tokens": 17,
                "prompt_tokens_details": {
                    "cached_tokens": 3,
                    "cache_write_tokens": 2
                },
                "completion_tokens_details": {
                    "reasoning_tokens": 4
                },
                "cost": 0.001
            }
        }))
        .expect("usage");

        assert_eq!(usage.prompt_tokens, 10);
        assert_eq!(usage.completion_tokens, 7);
        assert_eq!(usage.total_tokens, 17);
        assert_eq!(usage.cached_tokens, 3);
        assert_eq!(usage.cache_write_tokens, 2);
        assert_eq!(usage.reasoning_tokens, 4);
        assert_eq!(usage.cost, Some(0.001));
    }

    #[test]
    fn classifies_openrouter_failures() {
        assert_eq!(
            classify_http_error(429, "rate limited", "{}"),
            ModelErrorClass::RateLimited
        );
        assert_eq!(
            classify_http_error(503, "model is overloaded", "{}"),
            ModelErrorClass::Overloaded
        );
        assert_eq!(
            classify_http_error(401, "invalid api key", "{}"),
            ModelErrorClass::Auth
        );
        assert_eq!(
            classify_http_error(402, "insufficient credits", "{}"),
            ModelErrorClass::Billing
        );
        assert_eq!(
            classify_http_error(400, "context length exceeded", "{}"),
            ModelErrorClass::ContextLength
        );
        assert_eq!(
            classify_http_error(400, "invalid request", "{}"),
            ModelErrorClass::InvalidRequest
        );
        assert_eq!(
            classify_http_error(500, "internal error", "{}"),
            ModelErrorClass::Server
        );
    }

    #[tokio::test]
    async fn retries_rate_limited_request_then_streams_success() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::TOO_MANY_REQUESTS, "rate limited"),
            FakeResponse::sse("ok"),
        ])
        .await;
        let client = ChatModelClient::new(server.base_url(), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(2));

        let events = collect_events(client.stream(test_request("primary")).await.unwrap()).await;

        assert_eq!(server.request_count().await, 2);
        assert_eq!(
            events
                .into_iter()
                .filter_map(|event| match event {
                    ModelStreamEvent::TextDelta(text) => Some(text),
                    _ => None,
                })
                .collect::<Vec<_>>(),
            vec!["ok".to_string()]
        );
    }

    #[tokio::test]
    async fn does_not_retry_invalid_request() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::BAD_REQUEST, "invalid request body"),
            FakeResponse::sse("should not be used"),
        ])
        .await;
        let client = ChatModelClient::new(server.base_url(), "test-key")
            .with_retry_policy(ModelRetryPolicy::no_delay(3));

        let error = match client.stream(test_request("primary")).await {
            Ok(_) => panic!("invalid requests must fail immediately"),
            Err(error) => error,
        };

        assert_eq!(server.request_count().await, 1);
        assert!(error.to_string().contains("invalid_request"));
    }

    #[tokio::test]
    async fn falls_back_to_configured_model_after_transient_failures() {
        let server = FakeModelServer::spawn(vec![
            FakeResponse::json_error(StatusCode::SERVICE_UNAVAILABLE, "provider overloaded"),
            FakeResponse::json_error(StatusCode::SERVICE_UNAVAILABLE, "provider overloaded"),
            FakeResponse::sse("fallback ok"),
        ])
        .await;
        let primary_key = "MODEL_CLIENT_TEST_PRIMARY_KEY";
        let fallback_key = "MODEL_CLIENT_TEST_FALLBACK_KEY";
        let runtime_env = HashMap::from([
            (primary_key.to_string(), "primary-key".to_string()),
            (fallback_key.to_string(), "fallback-key".to_string()),
        ]);

        let model = ModelConfig::OpenaiCompatible {
            base_url: server.base_url(),
            model_id: "primary-model".to_string(),
            api_key_env: primary_key.to_string(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: Some(Box::new(ModelConfig::OpenaiCompatible {
                base_url: server.base_url(),
                model_id: "fallback-model".to_string(),
                api_key_env: fallback_key.to_string(),
                temperature: None,
                max_output_tokens: None,
                reasoning_effort: None,
                extra_headers: HashMap::new(),
                fallback: None,
            })),
        };
        let config = ChatModelClient::from_model_config(&model, &runtime_env).unwrap();
        let client = config
            .client
            .with_retry_policy(ModelRetryPolicy::no_delay(2));

        let events =
            collect_events(client.stream(test_request(&config.model_id)).await.unwrap()).await;

        assert_eq!(server.request_count().await, 3);
        assert_eq!(
            server.request_models().await,
            vec![
                "primary-model".to_string(),
                "primary-model".to_string(),
                "fallback-model".to_string(),
            ]
        );
        assert!(matches!(
            config.cache_policy,
            CacheControlPolicy::OpenRouterGeminiEphemeral
        ));
        assert!(events.iter().any(|event| matches!(
            event,
            ModelStreamEvent::TextDelta(text) if text == "fallback ok"
        )));
    }

    #[test]
    fn runtime_env_overlay_wins_for_model_api_key() {
        let key_name = format!(
            "TEST_RUNTIME_MODEL_KEY_{}",
            UNIX_EPOCH.elapsed().expect("system clock").as_nanos()
        );
        std::env::set_var(&key_name, "process-key");
        let model = ModelConfig::OpenaiCompatible {
            base_url: "https://example.com".to_string(),
            model_id: "test-model".to_string(),
            api_key_env: key_name.clone(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        };
        let mut runtime_env = HashMap::new();
        runtime_env.insert(key_name.clone(), "overlay-key".to_string());

        let config =
            ChatModelClient::from_model_config(&model, &runtime_env).expect("runtime env override");
        assert_eq!(config.client.endpoints[0].api_key, "overlay-key");
    }

    #[test]
    fn runtime_env_does_not_fall_back_to_process_for_model_api_key() {
        let key_name = format!(
            "TEST_RUNTIME_MODEL_KEY_FALLBACK_{}",
            UNIX_EPOCH.elapsed().expect("system clock").as_nanos()
        );
        std::env::set_var(&key_name, "process-only-key");
        let model = ModelConfig::OpenaiCompatible {
            base_url: "https://example.com".to_string(),
            model_id: "test-model".to_string(),
            api_key_env: key_name.clone(),
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            extra_headers: HashMap::new(),
            fallback: None,
        };

        let err = ChatModelClient::from_model_config(&model, &HashMap::new())
            .expect_err("process env fallback must not be used");
        assert!(err.to_string().contains("not set"));
    }

    fn test_request(model: &str) -> ModelRequest {
        ModelRequest {
            model: model.to_string(),
            messages: vec![AgentMessage::user("hello")],
            tools: vec![],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Disabled,
        }
    }

    async fn collect_events(mut stream: super::ModelEventStream) -> Vec<ModelStreamEvent> {
        let mut events = Vec::new();
        while let Some(event) = stream.next().await {
            events.push(event.unwrap());
        }
        events
    }

    struct FakeModelServer {
        addr: SocketAddr,
        state: Arc<FakeState>,
    }

    impl FakeModelServer {
        async fn spawn(responses: Vec<FakeResponse>) -> Self {
            let state = Arc::new(FakeState {
                responses: Mutex::new(VecDeque::from(responses)),
                requests: Mutex::new(Vec::new()),
            });
            let app = Router::new()
                .route("/chat/completions", post(fake_chat_completion))
                .with_state(state.clone());
            let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
            let addr = listener.local_addr().unwrap();
            tokio::spawn(async move {
                axum::serve(listener, app).await.unwrap();
            });
            Self { addr, state }
        }

        fn base_url(&self) -> String {
            format!("http://{}", self.addr)
        }

        async fn request_count(&self) -> usize {
            self.state.requests.lock().await.len()
        }

        async fn request_models(&self) -> Vec<String> {
            self.state
                .requests
                .lock()
                .await
                .iter()
                .filter_map(|request| request.get("model").and_then(|model| model.as_str()))
                .map(ToString::to_string)
                .collect()
        }
    }

    struct FakeState {
        responses: Mutex<VecDeque<FakeResponse>>,
        requests: Mutex<Vec<serde_json::Value>>,
    }

    #[derive(Clone)]
    struct FakeResponse {
        status: StatusCode,
        body: String,
    }

    impl FakeResponse {
        fn json_error(status: StatusCode, message: &str) -> Self {
            Self {
                status,
                body: json!({"error": {"message": message}}).to_string(),
            }
        }

        fn sse(text: &str) -> Self {
            Self {
                status: StatusCode::OK,
                body: format!(
                    "data: {{\"choices\":[{{\"delta\":{{\"content\":{}}}}}]}}\n\ndata: [DONE]\n\n",
                    serde_json::to_string(text).unwrap()
                ),
            }
        }
    }

    async fn fake_chat_completion(
        State(state): State<Arc<FakeState>>,
        Json(body): Json<serde_json::Value>,
    ) -> Response {
        state.requests.lock().await.push(body);
        let response = state.responses.lock().await.pop_front().unwrap_or_else(|| {
            FakeResponse::json_error(StatusCode::INTERNAL_SERVER_ERROR, "empty")
        });
        (
            response.status,
            [(header::CONTENT_TYPE, "text/event-stream")],
            response.body,
        )
            .into_response()
    }
}
