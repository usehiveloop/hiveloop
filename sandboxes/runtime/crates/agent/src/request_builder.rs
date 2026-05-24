use serde_json::{json, Value};

use crate::primitives::{
    AgentMessage, AgentMessageRole, CacheControlPolicy, MessagePart, ModelRequest, ToolCall,
};

pub fn build_openai_compatible_request(request: &ModelRequest) -> Value {
    let mut tools = request.tools.clone();
    tools.sort_by(|a, b| a.name.cmp(&b.name));

    let mut body = json!({
        "model": request.model,
        "messages": request.messages.iter().enumerate().map(|(i, m)| {
            let policy = if i == 0 && m.role == AgentMessageRole::System {
                request.cache_policy
            } else {
                CacheControlPolicy::Disabled
            };
            message_to_json(m, policy)
        }).collect::<Vec<_>>(),
        "stream": true,
        "stream_options": {"include_usage": true},
    });

    if !tools.is_empty() {
        body["tools"] = Value::Array(
            tools
                .into_iter()
                .map(|tool| {
                    json!({
                        "type": "function",
                        "function": {
                            "name": tool.name,
                            "description": tool.description,
                            "parameters": tool.parameters,
                        }
                    })
                })
                .collect(),
        );
        body["parallel_tool_calls"] = Value::Bool(true);
    }

    if let Some(temperature) = request.temperature {
        body["temperature"] = json!(temperature);
    }
    if let Some(max_tokens) = request.max_output_tokens {
        body["max_completion_tokens"] = json!(max_tokens);
    }
    if let Some(reasoning_effort) = &request.reasoning_effort {
        body["reasoning_effort"] = json!(reasoning_effort);
    }

    body
}

fn message_to_json(message: &AgentMessage, cache_policy: CacheControlPolicy) -> Value {
    match message.role {
        AgentMessageRole::System => json!({
            "role": "system",
            "content": parts_to_content(&message.parts, cache_policy),
        }),
        AgentMessageRole::User => json!({
            "role": "user",
            "content": parts_to_content(&message.parts, cache_policy),
        }),
        AgentMessageRole::Assistant => {
            let mut value = json!({"role": "assistant"});
            if !message.parts.is_empty() {
                value["content"] = parts_to_content(&message.parts, cache_policy);
            }
            if !message.tool_calls.is_empty() {
                value["tool_calls"] =
                    Value::Array(message.tool_calls.iter().map(tool_call_to_json).collect());
            }
            value
        }
        AgentMessageRole::Tool => json!({
            "role": "tool",
            "tool_call_id": message.tool_call_id.clone().unwrap_or_else(|| "unknown".to_string()),
            "content": parts_to_content(&message.parts, cache_policy),
        }),
    }
}

fn parts_to_content(parts: &[MessagePart], cache_policy: CacheControlPolicy) -> Value {
    Value::Array(
        parts
            .iter()
            .map(|part| match part {
                MessagePart::Text { text } => {
                    let mut value = json!({
                        "type": "text",
                        "text": text,
                    });
                    if cache_policy == CacheControlPolicy::OpenRouterGeminiEphemeral {
                        value["cache_control"] = json!({"type": "ephemeral"});
                    }
                    value
                }
                MessagePart::InlineData { mime_type, data } => {
                    json!({
                        "type": "image_url",
                        "image_url": {
                            "url": format!("data:{mime_type};base64,{}", base64_encode(data)),
                        }
                    })
                }
            })
            .collect(),
    )
}

fn tool_call_to_json(call: &ToolCall) -> Value {
    json!({
        "id": call.id,
        "type": "function",
        "function": {
            "name": call.name,
            "arguments": serde_json::to_string(&call.arguments).unwrap_or_else(|_| "{}".to_string()),
        }
    })
}

fn base64_encode(data: &[u8]) -> String {
    use base64::Engine;
    base64::engine::general_purpose::STANDARD.encode(data)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::primitives::{AgentMessage, CacheControlPolicy, ModelRequest};

    #[test]
    fn only_serializes_first_system_message_as_cacheable_content_block() {
        let req = ModelRequest {
            model: "test".into(),
            messages: vec![
                AgentMessage::system("sys"),
                AgentMessage::user("hi"),
                AgentMessage::assistant("hello"),
            ],
            tools: vec![],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::OpenRouterGeminiEphemeral,
        };
        let body = build_openai_compatible_request(&req);
        assert_eq!(body["messages"][0]["role"], "system");
        assert_eq!(body["messages"][1]["role"], "user");
        assert_eq!(body["messages"][2]["role"], "assistant");
        assert_eq!(
            body["messages"][0]["content"][0]["cache_control"]["type"],
            "ephemeral"
        );
        assert!(
            body["messages"][1]["content"][0]
                .as_object()
                .and_then(|o| o.get("cache_control"))
                .is_none(),
            "user messages must not have cache_control"
        );
        assert!(
            body["messages"][2]["content"][0]
                .as_object()
                .and_then(|o| o.get("cache_control"))
                .is_none(),
            "assistant messages must not have cache_control"
        );
    }

    #[test]
    fn sorts_tools_by_name() {
        let req = ModelRequest {
            model: "test".into(),
            messages: vec![AgentMessage::user("hi")],
            tools: vec![
                tools::ToolDefinition {
                    name: "z".into(),
                    description: "".into(),
                    parameters: json!({}),
                },
                tools::ToolDefinition {
                    name: "a".into(),
                    description: "".into(),
                    parameters: json!({}),
                },
            ],
            temperature: None,
            max_output_tokens: None,
            reasoning_effort: None,
            cache_policy: CacheControlPolicy::Disabled,
        };
        let body = build_openai_compatible_request(&req);
        assert_eq!(body["tools"][0]["function"]["name"], "a");
        assert_eq!(body["tools"][1]["function"]["name"], "z");
    }
}
