//! Per-conversation MCP server end-to-end tests.
//!
//! Covers the full HTTP → supervisor → McpManager path for MCP servers attached
//! at conversation creation time. Paired with unit tests in
//! `crates/runtime/src/supervisor.rs` (`per_conv_mcp_*`) that cover the
//! early-validation rejection paths without spinning up a real MCP connection.
//!
//! These tests use the stdio mock-portal-mcp binary, so the harness is started
//! with `BRIDGE_ALLOW_STDIO_MCP_FROM_API=true`.

use bridge_e2e::TestHarness;
use serde_json::json;
use std::time::Duration;

const TIMEOUT: Duration = Duration::from_secs(30);

/// Happy path: create a conversation with a stdio MCP server attached, send a
/// message through the full pipeline, end the conversation. A successful
/// create + stream + end proves every link in the chain:
///
/// - The API accepts `mcp_servers` in `CreateConversationRequest`
/// - The supervisor spawns and connects the stdio MCP process for this conv
/// - `list_tools` succeeds (otherwise `create_conversation` would unwind with
///   an `InvalidRequest`)
/// - The bridged tools are merged into the per-conversation executor map and
///   passed to rig-core (otherwise `build_agent` would fail)
/// - The conversation runs normally with the extended tool set
/// - Conversation end triggers the cleanup path that disconnects per-conv MCP
#[tokio::test]
async fn per_conv_mcp_happy_path_create_stream_end() {
    let harness = TestHarness::start_with_extra_env(&[("BRIDGE_ALLOW_STDIO_MCP_FROM_API", "true")])
        .await
        .expect("failed to start test harness with stdio MCP flag");

    let mcp_binary = harness
        .ensure_mock_portal_mcp()
        .expect("failed to build/locate mock-portal-mcp");
    let mcp_binary_path = mcp_binary.to_str().expect("binary path is utf-8").to_string();

    // Narrow the agent's base tool surface to just `bash`. mock-portal-mcp advertises
    // its own Glob/Grep/Read tools that would otherwise collide with the builtins,
    // which would fire the collision guard (itself tested below). By filtering the
    // base to a non-colliding tool, we isolate the happy-path assertions from the
    // collision path.
    let body = json!({
        "tool_names": ["bash"],
        "mcp_servers": [{
            "name": "per_conv_portal",
            "transport": {
                "type": "stdio",
                "command": mcp_binary_path,
                "args": [],
                "env": {}
            }
        }]
    });

    let resp = harness
        .create_conversation_with_body("agent_simple", body)
        .await
        .expect("create_conversation_with_body failed");
    let status = resp.status();
    let create_body: serde_json::Value =
        resp.json().await.expect("failed to parse create response");

    assert_eq!(
        status.as_u16(),
        201,
        "expected 201 Created, got {} body={}",
        status,
        create_body
    );

    let conversation_id = create_body
        .get("conversation_id")
        .and_then(|v| v.as_str())
        .expect("response should include conversation_id")
        .to_string();
    assert!(!conversation_id.is_empty());

    // Send a message and drain the stream to prove the turn completes cleanly
    // with the extended tool set in place.
    harness
        .register_conversation(&conversation_id, "agent_simple")
        .await;
    let send_resp = harness
        .send_message(&conversation_id, "hello")
        .await
        .expect("send_message failed");
    assert!(
        send_resp.status().is_success() || send_resp.status().as_u16() == 202,
        "expected 2xx from send_message, got {}",
        send_resp.status()
    );

    let (events, _response_text) = harness
        .stream_sse_until_done(&conversation_id, TIMEOUT)
        .await
        .expect("stream_sse_until_done failed");

    // The conversation must reach `done` without an `error` event — if any
    // part of the MCP wiring were broken the stream would surface it.
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "stream should include a done event; got events: {:?}",
        events.iter().map(|e| &e.event_type).collect::<Vec<_>>()
    );
    assert!(
        !events.iter().any(|e| e.event_type == "error"),
        "stream should not surface error events; got: {:?}",
        events
    );

    // Ending the conversation runs the cleanup block in `run_conversation`
    // which in turn calls `McpManager::disconnect_agent(conv_id)`. We don't
    // have direct visibility into the manager, but we can confirm the DELETE
    // handler returns 200 — any async disconnect error would have been logged
    // on the bridge side but not surfaced to the client, so the assertion here
    // is that the full shutdown path runs without panicking or blocking.
    let end_resp = harness
        .end_conversation(&conversation_id)
        .await
        .expect("end_conversation failed");
    assert_eq!(
        end_resp.status().as_u16(),
        200,
        "expected 200 OK from DELETE"
    );
}

/// Stdio rejection: with the flag disabled (the default), sending a stdio MCP
/// server definition from the API must return HTTP 400 — not spawn a process.
#[tokio::test]
async fn per_conv_mcp_stdio_rejected_when_flag_disabled() {
    // Default `start()` — no BRIDGE_ALLOW_STDIO_MCP_FROM_API in the env.
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let body = json!({
        "mcp_servers": [{
            "name": "should_fail",
            "transport": {
                "type": "stdio",
                "command": "/bin/echo",
                "args": [],
                "env": {}
            }
        }]
    });

    let resp = harness
        .create_conversation_with_body("agent_simple", body)
        .await
        .expect("create_conversation_with_body failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "expected 400 Bad Request when stdio flag is disabled"
    );
    let err_body: serde_json::Value = resp.json().await.unwrap_or_default();
    let msg = err_body
        .pointer("/error/message")
        .and_then(|v| v.as_str())
        .unwrap_or("");
    assert!(
        msg.contains("stdio transport not allowed"),
        "error message should explain stdio gate; got body: {}",
        err_body
    );
}

/// Duplicate server names in a single request must be rejected before any
/// connection attempt. Covers the dedup validation at the API boundary.
#[tokio::test]
async fn per_conv_mcp_duplicate_server_names_rejected() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let body = json!({
        "mcp_servers": [
            {
                "name": "dup",
                "transport": {
                    "type": "streamable_http",
                    "url": "http://127.0.0.1:1",
                    "headers": {}
                }
            },
            {
                "name": "dup",
                "transport": {
                    "type": "streamable_http",
                    "url": "http://127.0.0.1:2",
                    "headers": {}
                }
            }
        ]
    });

    let resp = harness
        .create_conversation_with_body("agent_simple", body)
        .await
        .expect("create_conversation_with_body failed");

    assert_eq!(resp.status().as_u16(), 400);
    let err_body: serde_json::Value = resp.json().await.unwrap_or_default();
    let msg = err_body
        .pointer("/error/message")
        .and_then(|v| v.as_str())
        .unwrap_or("");
    assert!(
        msg.contains("duplicate server name 'dup'"),
        "error should mention duplicate; got body: {}",
        err_body
    );
}

/// Collision rejection: mock-portal-mcp exposes tools named `Glob`, `Grep`,
/// and `Read` — which also exist as built-in tools on `agent_simple`. The
/// collision guard must catch this before the conversation handle is spawned.
#[tokio::test]
async fn per_conv_mcp_collision_with_builtin_rejected() {
    let harness = TestHarness::start_with_extra_env(&[("BRIDGE_ALLOW_STDIO_MCP_FROM_API", "true")])
        .await
        .expect("failed to start test harness with stdio MCP flag");

    let mcp_binary = harness
        .ensure_mock_portal_mcp()
        .expect("failed to build/locate mock-portal-mcp");
    let mcp_binary_path = mcp_binary.to_str().expect("binary path is utf-8").to_string();

    // No base tool filter → agent has Glob/Grep/Read built-ins → per-conv MCP
    // tries to register its own Glob/Grep/Read → collision guard fires.
    let body = json!({
        "mcp_servers": [{
            "name": "per_conv_portal",
            "transport": {
                "type": "stdio",
                "command": mcp_binary_path,
                "args": [],
                "env": {}
            }
        }]
    });

    let resp = harness
        .create_conversation_with_body("agent_simple", body)
        .await
        .expect("create_conversation_with_body failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "expected 400 Bad Request on tool collision"
    );
    let err_body: serde_json::Value = resp.json().await.unwrap_or_default();
    let msg = err_body
        .pointer("/error/message")
        .and_then(|v| v.as_str())
        .unwrap_or("");
    assert!(
        msg.contains("collides with an existing agent tool"),
        "error should explain collision; got body: {}",
        err_body
    );
}

/// An empty `mcp_servers` array is a valid no-op — the conversation should be
/// created as if the field were absent.
#[tokio::test]
async fn per_conv_mcp_empty_list_creates_normally() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let body = json!({ "mcp_servers": [] });

    let resp = harness
        .create_conversation_with_body("agent_simple", body)
        .await
        .expect("create_conversation_with_body failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "empty mcp_servers should not fail conversation creation"
    );
}
