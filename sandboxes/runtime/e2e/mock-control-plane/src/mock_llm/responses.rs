use super::types::{
    ChatCompletionResponse, Choice, FunctionCall, ResponseMessage, ToolCallResponse, Usage,
};

pub(super) fn build_text_response(user_message: &str, auth_header: &str) -> ChatCompletionResponse {
    let content = format!(
        "Mock LLM response to: {} [auth:{}]",
        user_message, auth_header
    );
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

pub(super) fn build_tool_call_response(tools: &[serde_json::Value]) -> ChatCompletionResponse {
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

pub(super) fn build_specific_tool_call_response(
    tool_name: &str,
    arguments: &str,
) -> ChatCompletionResponse {
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
                        arguments: arguments.to_string(),
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

/// Build a non-streaming response that calls the "agent" tool with proper arguments.
pub(super) fn build_agent_tool_call_response(
    subagent_name: &str,
    prompt: &str,
) -> ChatCompletionResponse {
    let args = serde_json::json!({
        "subagentName": subagent_name,
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
                        name: "sub_agent".to_string(),
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
