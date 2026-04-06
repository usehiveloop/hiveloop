//! CodeDB E2E tests — verify that BRIDGE_CODEDB_ENABLED replaces built-in
//! Grep/Read/Glob with codedb MCP tools and that a real LLM can use them
//! against the bridge codebase itself.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test codedb_e2e_tests -- --ignored
//! ```
//!
//! Requires `codedb` binary available (install via https://codedb.codegraff.com).

use bridge_e2e::{check, step, ConversationTurn, TestHarness};
use std::time::Duration;

const LLM_TIMEOUT: Duration = Duration::from_secs(300);

fn codedb_binary() -> String {
    std::env::var("CODEDB_BINARY").unwrap_or_else(|_| {
        let home = std::env::var("HOME").unwrap_or_default();
        format!("{}/bin/codedb", home)
    })
}

fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping codedb E2E test");
        return false;
    }
    true
}

fn require_codedb() -> bool {
    let binary = codedb_binary();
    if !std::path::Path::new(&binary).exists() {
        eprintln!("codedb binary not found at {} — skipping", binary);
        return false;
    }
    true
}

/// Assert that at least one codedb tool was called in the SSE events.
fn assert_codedb_tool_called(turn: &ConversationTurn, label: &str) {
    let codedb_tools = [
        "codedb_search",
        "codedb_read",
        "codedb_outline",
        "codedb_symbol",
        "codedb_tree",
        "codedb_word",
        "codedb_hot",
        "codedb_deps",
    ];

    let called_tools: Vec<String> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| {
            e.data
                .get("name")
                .and_then(|n| n.as_str())
                .map(String::from)
        })
        .collect();

    let found = called_tools
        .iter()
        .any(|t| codedb_tools.contains(&t.as_str()));
    check!(
        found,
        "[{}] at least one codedb tool should be called. Tools called: {:?}",
        label, called_tools
    );
}

/// Assert that none of the replaced built-in tools were called.
fn assert_no_builtin_search_tools(turn: &ConversationTurn, label: &str) {
    let replaced_tools = ["Grep", "Read", "Glob"];

    let called_tools: Vec<String> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| {
            e.data
                .get("name")
                .and_then(|n| n.as_str())
                .map(String::from)
        })
        .collect();

    for tool in &replaced_tools {
        check!(
            !called_tools.iter().any(|t| t == tool),
            "[{}] built-in {} tool should NOT be called (replaced by codedb). Tools called: {:?}",
            label, tool, called_tools
        );
    }
}

// ============================================================================
// Test 1: Agent uses codedb to find a specific function in the bridge codebase
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_codedb_finds_function_in_bridge() {
    if !require_fireworks_key() || !require_codedb() {
        return;
    }

    step!("Starting harness with codedb enabled (binary: {})", codedb_binary());
    let harness = TestHarness::start_with_codedb(&codedb_binary())
        .await
        .expect("failed to start codedb harness");

    step!("Sending message: 'Find the inject_codedb_if_enabled function...'");
    let turn = harness
        .converse(
            "codedb-agent",
            None,
            "Find the `inject_codedb_if_enabled` function in this codebase. What file is it in, what parameters does it take, and what does it do?",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {} {}", e.event_type, serde_json::to_string(&e.data).unwrap_or_default().chars().take(120).collect::<String>());
    }

    step!("Verifying response is non-empty");
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty. SSE events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );
    eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

    step!("Verifying codedb tools were called");
    assert_codedb_tool_called(&turn, "find-function");

    step!("Verifying no built-in search tools were called");
    assert_no_builtin_search_tools(&turn, "find-function");

    step!("Verifying response mentions supervisor/codedb");
    let response_lower = turn.response_text.to_lowercase();
    check!(
        response_lower.contains("supervisor") || response_lower.contains("codedb"),
        "response should mention supervisor.rs or codedb. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — found function via codedb in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 2: Agent uses codedb to explore the project structure
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_codedb_explores_bridge_structure() {
    if !require_fireworks_key() || !require_codedb() {
        return;
    }

    step!("Starting harness with codedb enabled (binary: {})", codedb_binary());
    let harness = TestHarness::start_with_codedb(&codedb_binary())
        .await
        .expect("failed to start codedb harness");

    step!("Sending message: 'What crates does this Rust workspace contain?'");
    let turn = harness
        .converse(
            "codedb-agent",
            None,
            "What crates does this Rust workspace contain? List each crate and the key structs or types it defines.",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {} {}", e.event_type, serde_json::to_string(&e.data).unwrap_or_default().chars().take(120).collect::<String>());
    }

    step!("Verifying response is non-empty");
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

    step!("Verifying codedb tools were called");
    assert_codedb_tool_called(&turn, "explore-structure");

    step!("Verifying no built-in search tools were called");
    assert_no_builtin_search_tools(&turn, "explore-structure");

    step!("Verifying response mentions known bridge crates");
    // Response should mention actual crates from the bridge workspace
    let response_lower = turn.response_text.to_lowercase();
    let known_crates = ["runtime", "tools", "api", "webhooks", "llm", "mcp"];
    let found_crates: Vec<&&str> = known_crates
        .iter()
        .filter(|c| response_lower.contains(**c))
        .collect();

    check!(
        found_crates.len() >= 3,
        "response should mention at least 3 bridge crates, found {:?}. Got: {}",
        found_crates,
        &turn.response_text[..turn.response_text.len().min(800)]
    );

    step!(
        "PASS — explored structure via codedb in {:?}, found crates: {:?}",
        turn.duration,
        found_crates
    );
}

// ============================================================================
// Test 3: Agent uses codedb to trace a code path across crates
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_codedb_traces_code_path() {
    if !require_fireworks_key() || !require_codedb() {
        return;
    }

    step!("Starting harness with codedb enabled (binary: {})", codedb_binary());
    let harness = TestHarness::start_with_codedb(&codedb_binary())
        .await
        .expect("failed to start codedb harness");

    step!("Sending message: 'How does AgentSupervisor register built-in tools...'");
    let turn = harness
        .converse(
            "codedb-agent",
            None,
            "How does the `AgentSupervisor` register built-in tools when loading an agent? Trace the code path from `load_single_agent` to the tool registration functions.",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {} {}", e.event_type, serde_json::to_string(&e.data).unwrap_or_default().chars().take(120).collect::<String>());
    }

    step!("Verifying response is non-empty");
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

    step!("Verifying codedb tools were called");
    assert_codedb_tool_called(&turn, "trace-path");

    step!("Verifying no built-in search tools were called");
    assert_no_builtin_search_tools(&turn, "trace-path");

    step!("Verifying response discusses tool registration");
    let response_lower = turn.response_text.to_lowercase();
    check!(
        response_lower.contains("register") && response_lower.contains("tool"),
        "response should discuss tool registration. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — traced code path via codedb in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 4: Agent with tools filter only gets allowed codedb tools
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_codedb_tools_filtered_by_agent_definition() {
    if !require_fireworks_key() || !require_codedb() {
        return;
    }

    step!("Starting harness with codedb enabled (binary: {})", codedb_binary());
    let harness = TestHarness::start_with_codedb(&codedb_binary())
        .await
        .expect("failed to start codedb harness");

    step!("Pushing filtered agent (only codedb_search + codedb_read allowed)");
    // Push a second agent that only allows codedb_search and codedb_read
    let fireworks_key = std::env::var("FIREWORKS_API_KEY").unwrap();
    let filtered_agent = serde_json::json!({
        "id": "codedb-filtered-agent",
        "name": "CodeDB Filtered Agent",
        "system_prompt": "You are a coding assistant. You have access to codedb_search and codedb_read tools. Use them to answer questions about code. Do NOT try to use tools that are not available to you.",
        "provider": {
            "provider_type": "open_ai",
            "model": "accounts/fireworks/models/kimi-k2p5",
            "api_key": fireworks_key,
            "base_url": "https://api.fireworks.ai/inference/v1"
        },
        "tools": [
            { "name": "codedb_search", "description": "search", "parameters_schema": {} },
            { "name": "codedb_read", "description": "read", "parameters_schema": {} },
            { "name": "bash", "description": "bash", "parameters_schema": {} }
        ],
        "config": {
            "max_tokens": 4096,
            "max_turns": 5,
            "temperature": 0.1
        }
    });

    harness
        .push_agent_to_bridge(&filtered_agent)
        .await
        .expect("failed to push filtered agent");
    tokio::time::sleep(Duration::from_secs(5)).await;

    step!("Sending message to filtered agent: 'Search for AgentSupervisor and read its file'");
    let turn = harness
        .converse(
            "codedb-filtered-agent",
            None,
            "Search for the word 'AgentSupervisor' in the codebase and read the file where it is defined.",
            LLM_TIMEOUT,
        )
        .await
        .expect("conversation failed");

    step!("SSE events ({} total)", turn.sse_events.len());
    for e in &turn.sse_events {
        eprintln!("    - {} {}", e.event_type, serde_json::to_string(&e.data).unwrap_or_default().chars().take(120).collect::<String>());
    }

    step!("Verifying response is non-empty");
    check!(
        !turn.response_text.is_empty(),
        "response should not be empty"
    );
    eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

    let called_tools: Vec<String> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .filter_map(|e| {
            e.data
                .get("name")
                .and_then(|n| n.as_str())
                .map(String::from)
        })
        .collect();

    step!("Tools called: {:?}", called_tools);

    step!("Verifying excluded codedb tools were NOT called");
    // Should only use codedb_search and/or codedb_read (the allowed tools)
    let excluded_codedb_tools = [
        "codedb_symbol",
        "codedb_outline",
        "codedb_tree",
        "codedb_word",
        "codedb_edit",
        "codedb_hot",
        "codedb_deps",
        "codedb_bundle",
        "codedb_remote",
        "codedb_snapshot",
        "codedb_changes",
        "codedb_status",
        "codedb_projects",
        "codedb_index",
    ];

    for tool in &excluded_codedb_tools {
        check!(
            !called_tools.iter().any(|t| t == tool),
            "excluded tool {} should NOT be called. Tools called: {:?}",
            tool,
            called_tools
        );
    }

    step!("Verifying allowed codedb tools were used");
    // Should have used at least one of the allowed codedb tools
    let used_allowed = called_tools
        .iter()
        .any(|t| t == "codedb_search" || t == "codedb_read");
    check!(
        used_allowed,
        "agent should have used codedb_search or codedb_read. Tools called: {:?}",
        called_tools
    );

    step!("Verifying no built-in search tools were called");
    assert_no_builtin_search_tools(&turn, "filtered");

    step!(
        "PASS — filtered agent only used allowed tools: {:?}, completed in {:?}",
        called_tools,
        turn.duration
    );
}
