use axum::http::StatusCode;
use axum::response::sse::{Event, KeepAlive, Sse};
use axum::response::IntoResponse;
use axum::Json;
use serde::{Deserialize, Serialize};
use std::convert::Infallible;
/// OpenAI-compatible chat completion request.
#[derive(Debug, Deserialize)]
pub struct ChatCompletionRequest {
    #[allow(dead_code)]
    pub model: Option<String>,
    pub messages: Vec<ChatMessage>,
    #[serde(default)]
    pub tools: Vec<serde_json::Value>,
    #[serde(default)]
    pub stream: bool,
}

/// A message in the chat completion request.
#[derive(Debug, Deserialize)]
pub struct ChatMessage {
    pub role: String,
    pub content: Option<String>,
}

/// OpenAI-compatible chat completion response.
#[derive(Debug, Serialize)]
pub struct ChatCompletionResponse {
    pub id: String,
    pub object: String,
    pub created: i64,
    pub model: String,
    pub choices: Vec<Choice>,
    pub usage: Usage,
}

/// A choice in the completion response.
#[derive(Debug, Serialize)]
pub struct Choice {
    pub index: u32,
    pub message: ResponseMessage,
    pub finish_reason: String,
}

/// The response message.
#[derive(Debug, Serialize)]
pub struct ResponseMessage {
    pub role: String,
    pub content: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tool_calls: Option<Vec<ToolCallResponse>>,
}

/// A tool call in the response.
#[derive(Debug, Serialize)]
pub struct ToolCallResponse {
    pub id: String,
    #[serde(rename = "type")]
    pub call_type: String,
    pub function: FunctionCall,
}

/// Function call details.
#[derive(Debug, Serialize)]
pub struct FunctionCall {
    pub name: String,
    pub arguments: String,
}

/// Token usage information.
#[derive(Debug, Serialize)]
pub struct Usage {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// Extract an agent tool trigger from the user message.
/// Pattern: "use_agent:SUBAGENT_NAME" in the message text.
/// Returns Some((subagent_name, cleaned_prompt)) if found.
fn extract_agent_trigger(message: &str) -> Option<(String, String)> {
    // Look for use_agent:NAME pattern
    let prefix = "use_agent:";
    if let Some(start) = message.find(prefix) {
        let after = &message[start + prefix.len()..];
        // Name is the next word (alphanumeric + underscore + hyphen)
        let name: String = after
            .chars()
            .take_while(|c| c.is_alphanumeric() || *c == '_' || *c == '-')
            .collect();
        if !name.is_empty() {
            // Build prompt from the message with the trigger removed
            let trigger = format!("{}{}", prefix, name);
            let prompt = message.replace(&trigger, "").trim().to_string();
            return Some((name, prompt));
        }
    }
    None
}

/// POST /v1/chat/completions — mock LLM endpoint.
pub async fn chat_completions(Json(req): Json<ChatCompletionRequest>) -> impl IntoResponse {
    let last_user_message = req
        .messages
        .iter()
        .rev()
        .find(|m| m.role == "user")
        .and_then(|m| m.content.as_deref())
        .unwrap_or("(no message)");

    // Check if there's already a tool result in the conversation.
    // If so, always return a text response (prevents infinite tool-call loops).
    let has_tool_result = req.messages.iter().any(|m| m.role == "tool");

    if has_tool_result {
        if req.stream {
            return stream_response(last_user_message, false, &req.tools).into_response();
        }
        return (StatusCode::OK, Json(build_text_response(last_user_message))).into_response();
    }

    // Check for agent tool trigger: use_agent:SUBAGENT_NAME
    if let Some((subagent_name, prompt)) = extract_agent_trigger(last_user_message) {
        let has_agent_tool = req.tools.iter().any(|t| {
            t.get("function")
                .and_then(|f| f.get("name"))
                .and_then(|n| n.as_str())
                == Some("agent")
        });

        if has_agent_tool {
            if req.stream {
                return stream_agent_tool_call(&subagent_name, &prompt).into_response();
            }
            return (
                StatusCode::OK,
                Json(build_agent_tool_call_response(&subagent_name, &prompt)),
            )
                .into_response();
        }
    }

    // If tools are provided and the user message contains "use_tool", return a tool call
    let should_call_tool =
        !req.tools.is_empty() && last_user_message.to_lowercase().contains("use_tool");

    if req.stream {
        return stream_response(last_user_message, should_call_tool, &req.tools).into_response();
    }

    // Non-streaming response
    let response = if should_call_tool {
        build_tool_call_response(&req.tools)
    } else {
        build_text_response(last_user_message)
    };

    (StatusCode::OK, Json(response)).into_response()
}

fn build_text_response(user_message: &str) -> ChatCompletionResponse {
    let content = format!("Mock LLM response to: {}", user_message);
    ChatCompletionResponse {
        id: format!("chatcmpl-{}", uuid::Uuid::new_v4()),
        object: "chat.completion".to_string(),
        created: chrono::Utc::now().timestamp(),
        model: "mock-model".to_string(),
        choices: vec![Choice {
            index: 0,
            message: ResponseMessage {
                role: "assistant".to_string(),
                content: Some(content),
                tool_calls: None,
            },
            finish_reason: "stop".to_string(),
        }],
        usage: Usage {
            prompt_tokens: 10,
            completion_tokens: 20,
            total_tokens: 30,
        },
    }
}

fn build_tool_call_response(tools: &[serde_json::Value]) -> ChatCompletionResponse {
    let tool_name = tools
        .first()
        .and_then(|t| t.get("function"))
        .and_then(|f| f.get("name"))
        .and_then(|n| n.as_str())
        .unwrap_or("unknown_tool");

    ChatCompletionResponse {
        id: format!("chatcmpl-{}", uuid::Uuid::new_v4()),
        object: "chat.completion".to_string(),
        created: chrono::Utc::now().timestamp(),
        model: "mock-model".to_string(),
        choices: vec![Choice {
            index: 0,
            message: ResponseMessage {
                role: "assistant".to_string(),
                content: None,
                tool_calls: Some(vec![ToolCallResponse {
                    id: format!("call_{}", uuid::Uuid::new_v4()),
                    call_type: "function".to_string(),
                    function: FunctionCall {
                        name: tool_name.to_string(),
                        arguments: "{}".to_string(),
                    },
                }]),
            },
            finish_reason: "tool_calls".to_string(),
        }],
        usage: Usage {
            prompt_tokens: 10,
            completion_tokens: 15,
            total_tokens: 25,
        },
    }
}

fn stream_response(
    user_message: &str,
    should_call_tool: bool,
    tools: &[serde_json::Value],
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    let content = if should_call_tool {
        None
    } else {
        Some(format!("Mock LLM response to: {}", user_message))
    };

    let tool_name = if should_call_tool {
        tools
            .first()
            .and_then(|t| t.get("function"))
            .and_then(|f| f.get("name"))
            .and_then(|n| n.as_str())
            .map(|s| s.to_string())
    } else {
        None
    };

    let stream = async_stream::stream! {
        let id = format!("chatcmpl-{}", uuid::Uuid::new_v4());

        if let Some(ref text) = content {
            // Stream text in chunks
            let words: Vec<&str> = text.split_whitespace().collect();
            for (i, word) in words.iter().enumerate() {
                let delta_content = if i == 0 {
                    word.to_string()
                } else {
                    format!(" {}", word)
                };

                let chunk = serde_json::json!({
                    "id": id,
                    "object": "chat.completion.chunk",
                    "created": chrono::Utc::now().timestamp(),
                    "model": "mock-model",
                    "choices": [{
                        "index": 0,
                        "delta": {
                            "role": "assistant",
                            "content": delta_content
                        },
                        "finish_reason": null
                    }]
                });

                yield Ok(Event::default().data(serde_json::to_string(&chunk).unwrap()));
            }

            // Final chunk with finish_reason
            let final_chunk = serde_json::json!({
                "id": id,
                "object": "chat.completion.chunk",
                "created": chrono::Utc::now().timestamp(),
                "model": "mock-model",
                "choices": [{
                    "index": 0,
                    "delta": {},
                    "finish_reason": "stop"
                }],
                "usage": {
                    "prompt_tokens": 10,
                    "completion_tokens": 20,
                    "total_tokens": 30
                }
            });
            yield Ok(Event::default().data(serde_json::to_string(&final_chunk).unwrap()));
        } else if let Some(ref name) = tool_name {
            // Stream a tool call
            let call_id = format!("call_{}", uuid::Uuid::new_v4());

            let chunk = serde_json::json!({
                "id": id,
                "object": "chat.completion.chunk",
                "created": chrono::Utc::now().timestamp(),
                "model": "mock-model",
                "choices": [{
                    "index": 0,
                    "delta": {
                        "role": "assistant",
                        "tool_calls": [{
                            "index": 0,
                            "id": call_id,
                            "type": "function",
                            "function": {
                                "name": name,
                                "arguments": "{}"
                            }
                        }]
                    },
                    "finish_reason": null
                }]
            });
            yield Ok(Event::default().data(serde_json::to_string(&chunk).unwrap()));

            let final_chunk = serde_json::json!({
                "id": id,
                "object": "chat.completion.chunk",
                "created": chrono::Utc::now().timestamp(),
                "model": "mock-model",
                "choices": [{
                    "index": 0,
                    "delta": {},
                    "finish_reason": "tool_calls"
                }],
                "usage": {
                    "prompt_tokens": 10,
                    "completion_tokens": 15,
                    "total_tokens": 25
                }
            });
            yield Ok(Event::default().data(serde_json::to_string(&final_chunk).unwrap()));
        }

        // Signal end of stream
        yield Ok(Event::default().data("[DONE]"));
    };

    Sse::new(stream).keep_alive(KeepAlive::default())
}

/// Build a non-streaming response that calls the "agent" tool with proper arguments.
fn build_agent_tool_call_response(subagent_name: &str, prompt: &str) -> ChatCompletionResponse {
    let args = serde_json::json!({
        "subagent": subagent_name,
        "prompt": prompt,
        "description": format!("delegating to {}", subagent_name)
    });

    ChatCompletionResponse {
        id: format!("chatcmpl-{}", uuid::Uuid::new_v4()),
        object: "chat.completion".to_string(),
        created: chrono::Utc::now().timestamp(),
        model: "mock-model".to_string(),
        choices: vec![Choice {
            index: 0,
            message: ResponseMessage {
                role: "assistant".to_string(),
                content: None,
                tool_calls: Some(vec![ToolCallResponse {
                    id: format!("call_{}", uuid::Uuid::new_v4()),
                    call_type: "function".to_string(),
                    function: FunctionCall {
                        name: "agent".to_string(),
                        arguments: serde_json::to_string(&args).unwrap(),
                    },
                }]),
            },
            finish_reason: "tool_calls".to_string(),
        }],
        usage: Usage {
            prompt_tokens: 10,
            completion_tokens: 15,
            total_tokens: 25,
        },
    }
}

/// Build a streaming response that calls the "agent" tool with proper arguments.
fn stream_agent_tool_call(
    subagent_name: &str,
    prompt: &str,
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    let args = serde_json::json!({
        "subagent": subagent_name,
        "prompt": prompt,
        "description": format!("delegating to {}", subagent_name)
    });
    let args_str = serde_json::to_string(&args).unwrap();

    let stream = async_stream::stream! {
        let id = format!("chatcmpl-{}", uuid::Uuid::new_v4());
        let call_id = format!("call_{}", uuid::Uuid::new_v4());

        let chunk = serde_json::json!({
            "id": id,
            "object": "chat.completion.chunk",
            "created": chrono::Utc::now().timestamp(),
            "model": "mock-model",
            "choices": [{
                "index": 0,
                "delta": {
                    "role": "assistant",
                    "tool_calls": [{
                        "index": 0,
                        "id": call_id,
                        "type": "function",
                        "function": {
                            "name": "agent",
                            "arguments": args_str
                        }
                    }]
                },
                "finish_reason": null
            }]
        });
        yield Ok(Event::default().data(serde_json::to_string(&chunk).unwrap()));

        let final_chunk = serde_json::json!({
            "id": id,
            "object": "chat.completion.chunk",
            "created": chrono::Utc::now().timestamp(),
            "model": "mock-model",
            "choices": [{
                "index": 0,
                "delta": {},
                "finish_reason": "tool_calls"
            }],
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 15,
                "total_tokens": 25
            }
        });
        yield Ok(Event::default().data(serde_json::to_string(&final_chunk).unwrap()));

        yield Ok(Event::default().data("[DONE]"));
    };

    Sse::new(stream).keep_alive(KeepAlive::default())
}
