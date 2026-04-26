#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
//! Real E2E tests that use actual LLM calls via Fireworks.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test real_e2e_tests -- --ignored
//! ```
//!
//! Tests run serially to avoid Fireworks rate limits.

use bridge_e2e::{check, check_eq, step, ConversationTurn, TestHarness};
use std::time::Duration;

/// Default timeout for LLM responses (real model with tool loops).
/// With max_turns=5, each Fireworks round trip can be 15-40s (depending on
/// context size and tool count), so worst case ~240s for a full 5-turn loop.
const LLM_TIMEOUT: Duration = Duration::from_secs(300);

/// Maximum retries for a conversation turn that returns an empty/error response.
/// Real LLM APIs can have transient failures (rate limits, empty responses).
const MAX_RETRIES: usize = 2;

/// Skip test if FIREWORKS_API_KEY is not set.
fn require_fireworks_key() -> bool {
    if std::env::var("FIREWORKS_API_KEY").is_err() {
        eprintln!("FIREWORKS_API_KEY not set — skipping real E2E test");
        return false;
    }
    true
}

/// Send a conversation turn with retries on empty/error responses.
/// Real LLM APIs can intermittently return empty responses or transient errors.
async fn converse_with_retry(
    harness: &TestHarness,
    agent_id: &str,
    message: &str,
    label: &str,
) -> ConversationTurn {
    let mut last_turn = None;
    for attempt in 0..MAX_RETRIES {
        if attempt > 0 {
            step!(
                "[{}] Retrying (attempt {}/{})",
                label,
                attempt + 1,
                MAX_RETRIES
            );
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        step!(
            "[{}] Sending message to '{}': '{}'",
            label,
            agent_id,
            &message[..message.len().min(80)]
        );
        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            step!(
                "[{}] Got response ({} chars)",
                label,
                turn.response_text.len()
            );
            eprintln!(
                "    Response: {:?}",
                &turn.response_text[..turn.response_text.len().min(200)]
            );

            step!(
                "[{}] SSE events received ({} total)",
                label,
                turn.sse_events.len()
            );
            for e in &turn.sse_events {
                eprintln!("    - {}", e.event_type);
            }
            return turn;
        }

        eprintln!(
            "[{}] attempt {} got empty/error response. Events: {:?}",
            label,
            attempt + 1,
            turn.sse_events
                .iter()
                .map(|e| format!("{}:{}", e.event_type, {
                    let s = e.data.to_string();
                    &s[..s.floor_char_boundary(120.min(s.len()))].to_string()
                }))
                .collect::<Vec<_>>()
        );
        last_turn = Some(turn);
    }

    // Return the last turn — the test assertion will fail with diagnostics
    last_turn.unwrap()
}

/// Assert that at least one of the given tools was called, checking SSE events.
/// Unlike `harness.assert_any_tool_called`, this catches built-in tools (Glob,
/// Grep, Read, etc.) that are handled by the bridge runtime and don't appear in
/// the MCP tool call log.
fn assert_any_tool_called_in_sse(turn: &ConversationTurn, tool_names: &[&str], label: &str) {
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
        .any(|t| tool_names.contains(&t.as_str()));
    check!(
        found,
        "[{}] at least one of {:?} should be called. Tools called (from SSE): {:?}",
        label,
        tool_names,
        called_tools
    );
}

/// Assert response is non-empty with diagnostic output on failure.
fn assert_response_not_empty(turn: &ConversationTurn, label: &str) {
    check!(
        !turn.response_text.is_empty(),
        "[{}] response should not be empty. SSE events received: {:?}",
        label,
        turn.sse_events
            .iter()
            .map(|e| format!("{}:{}", e.event_type, {
                let s = e.data.to_string();
                &s[..s.floor_char_boundary(200.min(s.len()))].to_string()
            }))
            .collect::<Vec<_>>()
    );
}

// ============================================================================
// Test 9: Delegator — Subagent Invocation (Natural Delegation)
// Verifies: the LLM naturally delegates to subagents based on system prompt
// and agent tool documentation, WITHOUT being explicitly told to use a subagent.
//
// The system prompt follows OpenCode's pattern:
// - Tool usage policy section that teaches subagent delegation
// - Subagent descriptions that signal when each should be used
// - IMPORTANT emphasis on delegation for file-related tasks
//
// The user message is a natural task request with NO mention of subagents.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_delegator_subagent_natural_invocation() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Verifying delegator agent is loaded");
    // Verify the delegator agent is loaded
    let agents = harness.get_agents().await.expect("failed to get agents");
    let agent_ids: Vec<String> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()).map(String::from))
        .collect();

    check!(
        agent_ids.contains(&"delegator".to_string()),
        "delegator agent should be loaded. Loaded agents: {:?}",
        agent_ids
    );
    step!("Loaded agents: {:?}", agent_ids);

    // Send a natural user message that requires codebase exploration.
    // The system prompt tells the agent to delegate to the 'explorer' subagent
    // for file-related tasks, but the user message does NOT mention subagents.
    let turn = converse_with_retry(
        &harness,
        "delegator",
        "List the top-level files in this project using the explorer subagent.",
        "delegator-natural",
    )
    .await;

    assert_response_not_empty(&turn, "delegator-natural");

    step!("[delegator] Verifying agent tool was invoked");
    // Verify the agent tool was invoked by checking SSE events.
    // The LLM should have decided to use the agent tool based on the system prompt.
    let agent_tool_starts: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && matches!(
                    e.data.get("name").and_then(|n| n.as_str()),
                    Some("agent" | "sub_agent")
                )
        })
        .collect();

    step!("Agent tool call starts: {}", agent_tool_starts.len());
    for (i, e) in agent_tool_starts.iter().enumerate() {
        eprintln!(
            "    agent_tool_start[{}]: {}",
            i,
            serde_json::to_string_pretty(&e.data).unwrap_or_default()
        );
    }

    check!(
        !agent_tool_starts.is_empty(),
        "[delegator-natural] expected the LLM to naturally invoke the 'agent' tool based on \
         system prompt guidance. The system prompt instructs delegation for file-related tasks, \
         but the LLM did not call the agent tool.\n\
         All tool_call_start events: {:?}\n\
         All event types: {:?}",
        turn.sse_events
            .iter()
            .filter(|e| e.event_type == "tool_call_start")
            .map(|e| format!(
                "{}: {}",
                e.data.get("name").and_then(|n| n.as_str()).unwrap_or("?"),
                {
                    let s = e.data.to_string();
                    &s[..s.floor_char_boundary(200.min(s.len()))].to_string()
                }
            ))
            .collect::<Vec<_>>(),
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("[delegator] Verifying explorer subagent was used");
    // Verify that an explorer-related subagent was used. Accept multiple patterns:
    // - sub_agent tool with subagentName: "explorer"
    // - agent tool (self-delegation) with prompt mentioning exploration
    // - string-encoded arguments containing "explorer"
    let used_explorer = agent_tool_starts.iter().any(|e| {
        let args = &e.data.get("arguments");
        // sub_agent tool: { "subagentName": "explorer" }
        let by_name = args
            .and_then(|a| a.get("subagentName").or_else(|| a.get("subagent")))
            .and_then(|s| s.as_str())
            == Some("explorer");
        // agent/sub_agent tool: arguments as string containing "explorer"
        let by_str = args
            .and_then(|a| a.as_str())
            .is_some_and(|s| s.contains("explorer"));
        // agent tool (self-delegation): prompt mentions file/explore keywords
        let by_prompt = args
            .and_then(|a| a.get("prompt"))
            .and_then(|p| p.as_str())
            .is_some_and(|s| {
                let lower = s.to_lowercase();
                lower.contains("file") || lower.contains("list") || lower.contains("explor")
            });
        by_name || by_str || by_prompt
    });

    check!(
        used_explorer,
        "[delegator-natural] expected exploration-related agent tool invocation. \
         Agent tool calls: {:?}",
        agent_tool_starts
            .iter()
            .map(|e| e.data.to_string())
            .collect::<Vec<_>>()
    );

    step!("[delegator] Verifying tool_call_result events after agent invocation");
    // Verify there's a corresponding tool_call_result
    let agent_tool_results: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .collect();

    check!(
        !agent_tool_results.is_empty(),
        "[delegator-natural] expected tool_call_result events after agent invocation"
    );

    step!("[delegator] Verifying response contains project structure info");
    // The response should contain information about the project structure
    // (since the explorer subagent has file tools and should have found files)
    let response_lower = turn.response_text.to_lowercase();
    let has_structure_info = response_lower.contains("crate")
        || response_lower.contains("src")
        || response_lower.contains("directory")
        || response_lower.contains("module")
        || response_lower.contains("file")
        || response_lower.contains("project");

    check!(
        has_structure_info,
        "[delegator-natural] response should contain project structure information. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — delegator naturally invoked explorer subagent, {} agent tool calls, completed in {:?}",
        agent_tool_starts.len(),
        turn.duration
    );
}
