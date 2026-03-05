mod catalog;
mod file_tools;
mod handlers;
mod mock_data;
mod protocol;

use protocol::{JsonRpcRequest, JsonRpcResponse, ToolCallParams, ToolResult};
use serde_json::json;
use std::io::{self, BufRead, Write};
use std::path::PathBuf;

fn main() {
    let workspace_dir = std::env::var("WORKSPACE_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| std::env::current_dir().unwrap_or_else(|_| PathBuf::from(".")));

    let log_file = std::env::var("PORTAL_MCP_LOG_FILE").ok();

    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut stdout_lock = stdout.lock();

    for line in stdin.lock().lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };

        let line = line.trim().to_string();
        if line.is_empty() {
            continue;
        }

        let request: JsonRpcRequest = match serde_json::from_str(&line) {
            Ok(r) => r,
            Err(e) => {
                let resp = JsonRpcResponse::error(None, -32700, format!("parse error: {e}"));
                write_response(&mut stdout_lock, &resp);
                continue;
            }
        };

        let response = handle_request(&request, &workspace_dir, log_file.as_deref());
        if let Some(resp) = response {
            write_response(&mut stdout_lock, &resp);
        }
    }
}

fn handle_request(
    req: &JsonRpcRequest,
    workspace_dir: &std::path::Path,
    log_file: Option<&str>,
) -> Option<JsonRpcResponse> {
    match req.method.as_str() {
        "initialize" => Some(JsonRpcResponse::success(
            req.id.clone(),
            json!({
                "protocolVersion": "2024-11-05",
                "capabilities": {
                    "tools": {}
                },
                "serverInfo": {
                    "name": "mock-portal-mcp",
                    "version": "1.0.0"
                }
            }),
        )),

        "notifications/initialized" => {
            // Client acknowledgement — no response needed
            None
        }

        "tools/list" => {
            let tools = catalog::tool_definitions();
            Some(JsonRpcResponse::success(
                req.id.clone(),
                json!({"tools": tools}),
            ))
        }

        "tools/call" => {
            let params: ToolCallParams = match serde_json::from_value(req.params.clone()) {
                Ok(p) => p,
                Err(e) => {
                    return Some(JsonRpcResponse::error(
                        req.id.clone(),
                        -32602,
                        format!("invalid params: {e}"),
                    ));
                }
            };

            let result = handlers::handle_tool_call(&params.name, &params.arguments, workspace_dir);

            // Log tool call to file
            if let Some(log_path) = log_file {
                log_tool_call(log_path, &params.name, &params.arguments, &result);
            }

            let result_value = serde_json::to_value(&result).unwrap_or(json!(null));
            Some(JsonRpcResponse::success(req.id.clone(), result_value))
        }

        "ping" => Some(JsonRpcResponse::success(req.id.clone(), json!({}))),

        _ => Some(JsonRpcResponse::error(
            req.id.clone(),
            -32601,
            format!("method not found: {}", req.method),
        )),
    }
}

fn write_response(stdout: &mut io::StdoutLock, resp: &JsonRpcResponse) {
    if let Ok(json) = serde_json::to_string(resp) {
        let _ = writeln!(stdout, "{json}");
        let _ = stdout.flush();
    }
}

fn log_tool_call(
    log_path: &str,
    tool_name: &str,
    arguments: &serde_json::Value,
    result: &ToolResult,
) {
    let entry = json!({
        "timestamp": chrono::Utc::now().to_rfc3339(),
        "tool_name": tool_name,
        "arguments": arguments,
        "result": {
            "content": result.content.iter().map(|c| json!({
                "type": c.content_type,
                "text": c.text
            })).collect::<Vec<_>>(),
            "isError": result.is_error
        }
    });

    if let Ok(line) = serde_json::to_string(&entry) {
        // Append to file; create if needed
        use std::fs::OpenOptions;
        if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(log_path) {
            let _ = writeln!(file, "{line}");
        }
    }
}
