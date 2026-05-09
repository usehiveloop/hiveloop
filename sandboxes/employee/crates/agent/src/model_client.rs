use std::pin::Pin;

use async_stream::stream;
use futures::{Stream, StreamExt};
use reqwest::Client;
use serde_json::Value;

use crate::primitives::{ModelRequest, ModelStreamEvent, ProviderUsage, ToolCall};
use crate::request_builder::build_openai_compatible_request;
use crate::{AgentError, Result};

pub type ModelEventStream = Pin<Box<dyn Stream<Item = Result<ModelStreamEvent>> + Send>>;

#[derive(Clone)]
pub struct ChatModelClient {
    http: Client,
    base_url: String,
    api_key: String,
}

impl ChatModelClient {
    pub fn new(base_url: impl Into<String>, api_key: impl Into<String>) -> Self {
        Self {
            http: Client::new(),
            base_url: base_url.into().trim_end_matches('/').to_string(),
            api_key: api_key.into(),
        }
    }

    pub async fn stream(&self, request: ModelRequest) -> Result<ModelEventStream> {
        let body = build_openai_compatible_request(&request);
        tracing::debug!(
            bytes = body.to_string().len(),
            tools = request.tools.len(),
            messages = request.messages.len(),
            "sending model request"
        );
        let response = self
            .http
            .post(format!("{}/chat/completions", self.base_url))
            .bearer_auth(&self.api_key)
            .json(&body)
            .send()
            .await
            .map_err(|e| AgentError::Model(format!("request: {e}")))?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().await.unwrap_or_default();
            return Err(AgentError::Model(format!("HTTP {status}: {text}")));
        }

        let bytes = response.bytes_stream();
        Ok(Box::pin(stream! {
            let mut buffer = String::new();
            let mut tool_accumulator = ToolCallAccumulator::default();
            futures::pin_mut!(bytes);
            while let Some(chunk) = bytes.next().await {
                let chunk = match chunk {
                    Ok(chunk) => chunk,
                    Err(error) => {
                        yield Err(AgentError::Model(format!("stream: {error}")));
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
                        let calls = tool_accumulator.finish();
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
                            if let Some(tool_calls) = delta.get("tool_calls").and_then(|v| v.as_array()) {
                                tool_accumulator.apply_delta(tool_calls);
                            }
                        }
                    }
                }
            }
        }))
    }
}

fn take_sse_event(buffer: &mut String) -> Option<String> {
    let idx = buffer.find("\n\n")?;
    let event = buffer[..idx].to_string();
    buffer.replace_range(..idx + 2, "");
    Some(event)
}

fn parse_usage(value: &Value) -> Option<ProviderUsage> {
    let usage = value.get("usage")?;
    let details = usage.get("prompt_tokens_details").unwrap_or(&Value::Null);
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
        cached_tokens: details
            .get("cached_tokens")
            .and_then(|v| v.as_i64())
            .unwrap_or_default(),
        cache_write_tokens: details
            .get("cache_write_tokens")
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

    fn finish(self) -> Vec<ToolCall> {
        self.calls
            .into_iter()
            .filter(|call| !call.name.is_empty())
            .map(|call| ToolCall {
                id: if call.id.is_empty() {
                    format!("tool_{}", call.name)
                } else {
                    call.id
                },
                name: call.name,
                arguments: serde_json::from_str(&call.arguments)
                    .unwrap_or_else(|_| serde_json::json!({})),
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
