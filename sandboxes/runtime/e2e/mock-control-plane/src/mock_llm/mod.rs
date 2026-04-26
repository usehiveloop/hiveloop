mod responses;
mod streams;
mod triggers;
mod types;

use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;

use responses::{
    build_agent_tool_call_response, build_specific_tool_call_response, build_text_response,
    build_tool_call_response,
};
use streams::{stream_agent_tool_call, stream_response, stream_specific_tool_call};
use triggers::{extract_agent_trigger, extract_integration_trigger, mock_integration_args};
use types::ChatCompletionRequest;

/// POST /v1/chat/completions — mock LLM endpoint.
pub async fn chat_completions(
    headers: axum::http::HeaderMap,
    Json(req): Json<ChatCompletionRequest>,
) -> impl IntoResponse {
    let auth_header = headers
        .get("authorization")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("none");

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
            return stream_response(last_user_message, false, &req.tools, auth_header)
                .into_response();
        }
        return (
            StatusCode::OK,
            Json(build_text_response(last_user_message, auth_header)),
        )
            .into_response();
    }

    // Check for integration tool trigger: use_integration:INTEGRATION:ACTION
    if let Some((tool_name, _prompt)) = extract_integration_trigger(last_user_message) {
        let has_integration_tool = req.tools.iter().any(|t| {
            t.get("function")
                .and_then(|f| f.get("name"))
                .and_then(|n| n.as_str())
                == Some(&tool_name)
        });

        if has_integration_tool {
            let args = mock_integration_args(&tool_name);
            let args_str = serde_json::to_string(&args).unwrap();

            if req.stream {
                return stream_specific_tool_call(&tool_name, &args_str).into_response();
            }
            return (
                StatusCode::OK,
                Json(build_specific_tool_call_response(&tool_name, &args_str)),
            )
                .into_response();
        }
    }

    // Check for agent tool trigger: use_agent:SUBAGENT_NAME
    if let Some((subagent_name, prompt)) = extract_agent_trigger(last_user_message) {
        let has_agent_tool = req.tools.iter().any(|t| {
            t.get("function")
                .and_then(|f| f.get("name"))
                .and_then(|n| n.as_str())
                == Some("sub_agent")
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
        return stream_response(last_user_message, should_call_tool, &req.tools, auth_header)
            .into_response();
    }

    // Non-streaming response
    let response = if should_call_tool {
        build_tool_call_response(&req.tools)
    } else {
        build_text_response(last_user_message, auth_header)
    };

    (StatusCode::OK, Json(response)).into_response()
}
