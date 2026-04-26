use axum::response::sse::{Event, KeepAlive, Sse};
use std::convert::Infallible;

pub(super) fn stream_specific_tool_call(
    tool_name: &str,
    arguments: &str,
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    let tool_name = tool_name.to_string();
    let arguments = arguments.to_string();

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
                            "name": tool_name,
                            "arguments": arguments
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

pub(super) fn stream_response(
    user_message: &str,
    should_call_tool: bool,
    tools: &[serde_json::Value],
    auth_header: &str,
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    let content = if should_call_tool {
        None
    } else {
        Some(format!(
            "Mock LLM response to: {} [auth:{}]",
            user_message, auth_header
        ))
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

/// Build a streaming response that calls the "sub_agent" tool with proper arguments.
pub(super) fn stream_agent_tool_call(
    subagent_name: &str,
    prompt: &str,
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    let args = serde_json::json!({
        "subagentName": subagent_name,
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
                            "name": "sub_agent",
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
