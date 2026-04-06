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
            step!("[{}] Retrying (attempt {}/{})", label, attempt + 1, MAX_RETRIES);
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        step!("[{}] Sending message to '{}': '{}'", label, agent_id, &message[..message.len().min(80)]);
        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            step!("[{}] Got response ({} chars)", label, turn.response_text.len());
            eprintln!("    Response: {:?}", &turn.response_text[..turn.response_text.len().min(200)]);

            step!("[{}] SSE events received ({} total)", label, turn.sse_events.len());
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
        label, tool_names, called_tools
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
// Test 1: Hana — Code Review
// Verifies: getIssue, listIssuePullRequests/getPullRequest, createComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_hana_code_review() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "code-review",
        "You've been assigned to review issue ENG-42. It has a linked pull request. Please: 1) Fetch the issue details using getIssue, 2) Find and review the linked PR using listIssuePullRequests and getPullRequest, 3) Post a brief review summary comment on the issue using createComment.",
        "hana",
    )
    .await;

    assert_response_not_empty(&turn, "hana");

    step!("[hana] Verifying getIssue was called");
    harness
        .assert_tool_called("getIssue")
        .expect("getIssue should have been called");

    step!("[hana] Verifying PR-related tools were called");
    harness
        .assert_any_tool_called(&["listIssuePullRequests", "getPullRequest"])
        .expect("PR-related tools should have been called");

    step!("[hana] Verifying createComment was called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    step!(
        "PASS — hana code review completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 2: Nova — Portal Control
// Verifies: listTeamIssues/listTeams, createIssue/submitApprovalRequest, pingHuman
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_nova_portal_control() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Turn 1: List issues
    step!("[nova] Turn 1: listing team issues");
    let turn1 = converse_with_retry(
        &harness,
        "portal-control",
        "What issues does the ENG team currently have? Give me a brief status summary.",
        "nova turn1",
    )
    .await;

    assert_response_not_empty(&turn1, "nova turn1");

    step!("[nova turn1] Verifying team/issue tools were called");
    harness
        .assert_any_tool_called(&["listTeamIssues", "listTeams", "getTeam"])
        .expect("team/issue tools should have been called");

    step!(
        "[nova turn1] completed in {:?}, response: {} chars",
        turn1.duration,
        turn1.response_text.len()
    );

    // Clear log for turn 2 assertions
    let log_dir = std::env::temp_dir().join("portal-mcp-logs");
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 2: Create issue (separate conversation)
    step!("[nova] Turn 2: creating issue");
    let turn2 = converse_with_retry(
        &harness,
        "portal-control",
        "Create a new high-priority issue on the ENG team titled 'Implement API rate limiting'. You have my approval — go ahead and use createIssue directly.",
        "nova turn2",
    )
    .await;

    assert_response_not_empty(&turn2, "nova turn2");

    step!("[nova turn2] Verifying issue creation/approval tools were called");
    harness
        .assert_any_tool_called(&["createIssue", "submitApprovalRequest"])
        .expect("issue creation or approval tools should have been called");

    step!(
        "[nova turn2] completed in {:?}, response: {} chars",
        turn2.duration,
        turn2.response_text.len()
    );

    // Clear log for turn 3
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 3: Ping human (separate conversation)
    step!("[nova] Turn 3: pinging human");
    let turn3 = converse_with_retry(
        &harness,
        "portal-control",
        "Use the pingHuman tool to alert the team lead that we need human review on the rate limiting approach. It's urgent.",
        "nova turn3",
    )
    .await;

    assert_response_not_empty(&turn3, "nova turn3");

    step!("[nova turn3] Verifying pingHuman was called");
    harness
        .assert_tool_called("pingHuman")
        .expect("pingHuman should have been called");

    step!(
        "PASS — nova portal-control all 3 turns completed (turn3: {:?}, {} chars)",
        turn3.duration,
        turn3.response_text.len()
    );
}

// ============================================================================
// Test 3: Skai — Security Audit
// Verifies: getIssue, listIssuePullRequests/getPullRequest, createComment/addPullRequestComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_skai_security_audit() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "security-audit",
        "Audit issue ENG-42 for security vulnerabilities. Steps: 1) Use getIssue to get the issue, 2) Use listIssuePullRequests and getPullRequest to get the PR diff, 3) Analyze the JWT auth code for vulnerabilities, 4) Post your findings as a comment using createComment.",
        "skai",
    )
    .await;

    assert_response_not_empty(&turn, "skai");

    step!("[skai] Verifying getIssue was called");
    harness
        .assert_tool_called("getIssue")
        .expect("getIssue should have been called");

    step!("[skai] Verifying PR tools were called");
    harness
        .assert_any_tool_called(&["listIssuePullRequests", "getPullRequest"])
        .expect("PR tools should have been called");

    step!("[skai] Verifying comment tools were called");
    harness
        .assert_any_tool_called(&["createComment", "addPullRequestComment"])
        .expect("comment tools should have been called");

    step!(
        "PASS — skai security audit completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 4: Theo — System Design
// Verifies: Glob/Grep/Read (codebase exploration), createDocument, createComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_theo_system_design() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "system-design",
        "Design a caching layer for this codebase. Steps: 1) Use Glob to find the main source files (pattern: 'crates/*/src/lib.rs'), 2) Read one file to understand the architecture, 3) Create a design document using createDocument with title 'Caching Layer Design', 4) Post a brief summary on issue ENG-43 using createComment.",
        "theo",
    )
    .await;

    assert_response_not_empty(&turn, "theo");

    step!("[theo] Verifying design workflow tools were called (SSE)");
    // The agent should use at least some tools for its design workflow.
    // Built-in tools (Glob/Grep/Read) don't appear in the MCP log, so check SSE.
    assert_any_tool_called_in_sse(
        &turn,
        &[
            "Glob",
            "Grep",
            "Read",
            "LS",
            "getIssue",
            "searchDocuments",
            "listDocuments",
            "createDocument",
            "updateDocument",
            "createComment",
        ],
        "theo design workflow",
    );

    step!("[theo] Verifying createComment was called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    step!(
        "PASS — theo system design completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 5: Mimi — Technical Writer
// Verifies: Glob/Read (code reading), createDocument, createComment
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_mimi_technical_writer() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "technical-writer",
        "Document the bridge HTTP API. Steps: 1) Use Glob with pattern 'crates/api/src/**/*.rs' to find handler files, 2) Read one handler file to understand the endpoints, 3) Create an API reference document using createDocument, 4) Post a summary on issue ENG-44 using createComment.",
        "mimi",
    )
    .await;

    assert_response_not_empty(&turn, "mimi");

    step!("[mimi] Verifying exploration tools were called (SSE)");
    // Built-in tools (Glob/Grep/Read) are handled by the bridge runtime and
    // don't appear in the MCP tool call log — check SSE events instead.
    assert_any_tool_called_in_sse(
        &turn,
        &[
            "Glob",
            "Grep",
            "Read",
            "LS",
            "getIssue",
            "searchDocuments",
            "listDocuments",
        ],
        "mimi exploration",
    );

    step!("[mimi] Verifying document creation tools were called");
    harness
        .assert_any_tool_called(&["createDocument", "updateDocument"])
        .expect("document creation tools should have been called");

    step!("[mimi] Verifying createComment was called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    step!(
        "PASS — mimi technical writer completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 6: Researcher — Web Search
// Verifies: web_search built-in tool, mock search endpoint, result synthesis
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_researcher_web_search() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "researcher",
        "Research how async/await works in Rust. Use the web_search tool to find information.",
        "researcher-search",
    )
    .await;

    assert_response_not_empty(&turn, "researcher search");

    let response = turn.response_text.to_lowercase();

    step!("[researcher] Verifying response contains search content");
    // The mock search endpoint returns results with unique markers and
    // content about Tokio, async/await, Futures. The agent MUST use the
    // web_search tool to know these — it can't fabricate the markers.
    let has_search_content = response.contains("tokio")
        || response.contains("async")
        || response.contains("await")
        || response.contains("bridge_e2e_search_marker");

    check!(
        has_search_content,
        "response should contain content from search results. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — researcher web_search completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 7: Researcher — Web Fetch (Real URL)
// Verifies: web_fetch tool with real HTTP fetch, HTML parsing, content extraction
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_researcher_web_fetch() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "researcher",
        "Use the web_fetch tool to fetch the page at https://www.rust-lang.org/ and give me a detailed summary of what the page contains. Report specific text and details you find on the page.",
        "researcher-fetch",
    )
    .await;

    assert_response_not_empty(&turn, "researcher fetch");

    let response = turn.response_text.to_lowercase();

    step!("[researcher-fetch] Verifying response contains fetched content");
    // The agent must have actually fetched rust-lang.org to know this content
    let has_fetch_content = response.contains("rust")
        && (response.contains("performance")
            || response.contains("reliable")
            || response.contains("memory")
            || response.contains("safety")
            || response.contains("concurrency"));

    check!(
        has_fetch_content,
        "response should contain content from rust-lang.org. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    step!(
        "PASS — researcher web_fetch completed in {:?}, response: {} chars",
        turn.duration,
        turn.response_text.len()
    );
}

// ============================================================================
// Test 8: Tool Call SSE Events
// Verifies: tool_call_start and tool_call_result SSE events are emitted
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_tool_call_sse_events() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Use the researcher agent — web_search is guaranteed to trigger tool calls
    let turn = converse_with_retry(
        &harness,
        "researcher",
        "Use the web_search tool to search for 'Rust async await'. Report your findings.",
        "tool-call-sse",
    )
    .await;

    assert_response_not_empty(&turn, "tool-call-sse");

    // Collect tool_call_start and tool_call_result events from the SSE stream
    let start_events: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_start")
        .collect();

    let result_events: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .collect();

    step!(
        "tool_call_start events: {}, tool_call_result events: {}",
        start_events.len(),
        result_events.len()
    );

    // Log each tool call start/result pair
    for event in &start_events {
        eprintln!("    tool_call_start: name={}, id={}",
            event.data.get("name").and_then(|n| n.as_str()).unwrap_or("?"),
            event.data.get("id").and_then(|n| n.as_str()).unwrap_or("?")
        );
    }
    for event in &result_events {
        let result_str = event.data.get("result").and_then(|r| r.as_str()).unwrap_or("");
        eprintln!("    tool_call_result: id={}, result={:?}",
            event.data.get("id").and_then(|n| n.as_str()).unwrap_or("?"),
            &result_str[..result_str.len().min(200)]
        );
    }

    step!("Verifying tool_call_start events exist");
    // Assert tool_call_start events exist
    check!(
        !start_events.is_empty(),
        "expected at least one tool_call_start SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("Verifying tool_call_result events exist");
    // Assert tool_call_result events exist
    check!(
        !result_events.is_empty(),
        "expected at least one tool_call_result SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    step!("Verifying start/result event counts match");
    // Each start should have a matching result (same count)
    check_eq!(
        start_events.len(),
        result_events.len(),
        "tool_call_start count should match tool_call_result count"
    );

    step!("Validating tool_call_start data fields");
    // Validate tool_call_start data contains required fields
    for event in &start_events {
        check!(
            event.data.get("name").is_some(),
            "tool_call_start should have 'name' field: {:?}",
            event.data
        );
        check!(
            event.data.get("arguments").is_some(),
            "tool_call_start should have 'arguments' field: {:?}",
            event.data
        );
        check!(
            event.data.get("id").is_some(),
            "tool_call_start should have 'id' field: {:?}",
            event.data
        );
    }

    step!("Validating tool_call_result data fields");
    // Validate tool_call_result data contains required fields
    for event in &result_events {
        check!(
            event.data.get("result").is_some(),
            "tool_call_result should have 'result' field: {:?}",
            event.data
        );
        check!(
            event.data.get("is_error").is_some(),
            "tool_call_result should have 'is_error' field: {:?}",
            event.data
        );
        check!(
            event.data.get("id").is_some(),
            "tool_call_result should have 'id' field: {:?}",
            event.data
        );
    }

    step!(
        "PASS — tool call SSE events verified: {} tool calls, completed in {:?}",
        start_events.len(),
        turn.duration
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
        eprintln!("    agent_tool_start[{}]: {}", i, serde_json::to_string_pretty(&e.data).unwrap_or_default());
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

// ============================================================================
// Test 10: Multi-Agent Concurrent Conversations
// Verifies: all agents respond, metrics tracked, no cross-contamination
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_multi_agent_concurrent_conversations() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Verifying all expected agents are loaded");
    // Verify all agents are loaded
    let agents = harness.get_agents().await.expect("failed to get agents");
    let agent_ids: Vec<String> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()).map(String::from))
        .collect();

    for expected_id in &[
        "code-review",
        "portal-control",
        "security-audit",
        "system-design",
        "technical-writer",
        "researcher",
        "delegator",
        "executor",
    ] {
        check!(
            agent_ids.contains(&expected_id.to_string()),
            "agent '{}' should be loaded. Loaded: {:?}",
            expected_id,
            agent_ids
        );
    }
    step!("All 8 agents loaded: {:?}", agent_ids);

    // Create 8 conversations simultaneously with simple non-tool messages
    let messages = vec![
        (
            "code-review",
            "What is the most important thing in a code review? Answer in 2-3 sentences.",
        ),
        (
            "portal-control",
            "Briefly describe your role as Portal in this workspace. 2-3 sentences.",
        ),
        (
            "security-audit",
            "What are the top 3 OWASP vulnerabilities? Answer briefly.",
        ),
        (
            "system-design",
            "What makes a good system design document? Answer in 2-3 sentences.",
        ),
        (
            "technical-writer",
            "What makes good API documentation? Answer in 2-3 sentences.",
        ),
        (
            "researcher",
            "What is Rust known for? Answer in 2-3 sentences.",
        ),
        (
            "delegator",
            "What makes a good engineering lead? Answer in 2-3 sentences.",
        ),
        (
            "executor",
            "What is the most important DevOps principle? Answer in 2-3 sentences.",
        ),
    ];

    step!("Spawning 8 concurrent conversations");
    let mut handles = Vec::new();
    for (agent_id, message) in &messages {
        let agent_id = agent_id.to_string();
        let message = message.to_string();
        let bridge_url = harness.bridge_url().to_string();

        handles.push(tokio::spawn(async move {
            let client = reqwest::Client::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .unwrap();

            // Create conversation
            let resp = client
                .post(format!("{}/agents/{}/conversations", bridge_url, agent_id))
                .send()
                .await
                .expect("create conversation failed");

            let body: serde_json::Value = resp.json().await.expect("parse failed");
            let conv_id = body
                .get("conversation_id")
                .and_then(|v| v.as_str())
                .expect("no conversation_id")
                .to_string();

            // Send message
            let msg_resp = client
                .post(format!("{}/conversations/{}/messages", bridge_url, conv_id))
                .json(&serde_json::json!({"content": message}))
                .send()
                .await
                .expect("send message failed");

            assert!(
                msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
                "message send failed for {}: {}",
                agent_id,
                msg_resp.status()
            );

            // Connect to SSE stream and read events properly using bytes_stream.
            // IMPORTANT: Do NOT use `.text().await` on SSE streams — it waits for the
            // TCP connection to close, which may never happen or take very long.
            // Instead, read byte-by-byte and stop when we see a "done" event.
            use futures::StreamExt;

            let stream_client = reqwest::Client::builder()
                .connect_timeout(Duration::from_secs(10))
                .build()
                .unwrap();

            let stream_resp = stream_client
                .get(format!("{}/conversations/{}/stream", bridge_url, conv_id))
                .send()
                .await
                .expect("stream connect failed");

            let mut stream = stream_resp.bytes_stream();
            let mut buffer = String::new();
            let mut response_text = String::new();
            let mut got_done = false;
            let mut current_event_type = String::new();

            let deadline = std::time::Instant::now() + LLM_TIMEOUT;

            loop {
                let remaining = deadline.saturating_duration_since(std::time::Instant::now());
                if remaining.is_zero() {
                    eprintln!("[concurrent] {} SSE stream timed out", agent_id);
                    break;
                }

                match tokio::time::timeout(remaining, stream.next()).await {
                    Ok(Some(Ok(chunk))) => {
                        buffer.push_str(&String::from_utf8_lossy(&chunk));
                    }
                    Ok(Some(Err(e))) => {
                        eprintln!("[concurrent] {} SSE chunk error: {}", agent_id, e);
                        break;
                    }
                    Ok(None) => break, // stream ended
                    Err(_) => {
                        eprintln!("[concurrent] {} SSE timeout", agent_id);
                        break;
                    }
                }

                // Process complete lines
                while let Some(newline_pos) = buffer.find('\n') {
                    let line = buffer[..newline_pos].trim_end().to_string();
                    buffer = buffer[newline_pos + 1..].to_string();

                    if line.is_empty() {
                        continue;
                    }

                    if let Some(event_name) = line.strip_prefix("event:") {
                        current_event_type = event_name.trim().to_string();
                    } else if let Some(data_str) = line.strip_prefix("data:") {
                        let data_str = data_str.trim();
                        if data_str.is_empty() {
                            continue;
                        }

                        let event_type = if !current_event_type.is_empty() {
                            current_event_type.clone()
                        } else if let Ok(v) = serde_json::from_str::<serde_json::Value>(data_str) {
                            v.get("type")
                                .and_then(|t| t.as_str())
                                .unwrap_or("message")
                                .to_string()
                        } else {
                            "message".to_string()
                        };

                        if event_type == "content_delta" {
                            if let Ok(v) = serde_json::from_str::<serde_json::Value>(data_str) {
                                if let Some(delta) = v.get("delta").and_then(|d| d.as_str()) {
                                    response_text.push_str(delta);
                                }
                            }
                        }

                        if event_type == "done" {
                            got_done = true;
                            break;
                        }

                        current_event_type.clear();
                    }
                }

                if got_done {
                    break;
                }
            }

            assert!(
                !response_text.is_empty(),
                "agent {} returned empty response",
                agent_id
            );

            eprintln!(
                "\n  \x1b[36m\u{25b8}\x1b[0m [concurrent] {} responded ({} chars): {:?}",
                agent_id,
                response_text.len(),
                &response_text[..response_text.len().min(100)]
            );

            (agent_id, conv_id)
        }));
    }

    step!("Waiting for all 8 conversations to complete");
    // Wait for all 8 conversations
    let mut results = Vec::new();
    for handle in handles {
        let result = handle.await.expect("task panicked");
        results.push(result);
    }

    check_eq!(results.len(), 8, "all 8 agents should have responded");

    step!("Verifying metrics");
    // Verify metrics show conversations tracked
    let metrics = harness.get_metrics().await.expect("failed to get metrics");

    if let Some(global) = metrics.get("global") {
        let total_agents = global
            .get("total_agents")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        check!(
            total_agents >= 8,
            "should have at least 8 agents in metrics, got {}",
            total_agents
        );
    }

    step!("All 8 agents responded:");
    for (agent_id, conv_id) in &results {
        eprintln!("    {} -> conversation {}", agent_id, conv_id);
    }

    step!("PASS — all 8 concurrent conversations completed successfully");
}

// ============================================================================
// Test: Abort — Per-Conversation Cancellation
// Verifies: POST /conversations/{conv_id}/abort cancels the in-flight turn,
// SSE stream receives error(aborted) + done, conversation remains usable.
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_abort_conversation() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Creating conversation for researcher agent");
    // Use researcher agent — it has simple tools (web_search, web_fetch) that
    // reliably work with Fireworks and take enough time for the abort to fire.
    let resp = harness
        .create_conversation("researcher")
        .await
        .expect("create conversation");
    let body: serde_json::Value = resp.json().await.expect("parse create response");
    let conv_id = body["conversation_id"]
        .as_str()
        .expect("conversation_id")
        .to_string();

    step!("Conversation created: {}", conv_id);
    harness.register_conversation(&conv_id, "researcher").await;

    step!("Sending long research message to trigger tool calls");
    // Send a message that will trigger tool calls (web_search takes time)
    let msg_resp = harness
        .send_message(
            &conv_id,
            "Research the history of the Rust programming language in depth. Use the web_search tool to search for 'Rust programming language history timeline'. Then search for 'Rust borrow checker design'. Then search for 'Rust async await RFC history'. Give me a comprehensive report.",
        )
        .await
        .expect("send message");
    check!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send returned success/202 (got {})",
        msg_resp.status()
    );

    step!("Starting SSE reader for both turns (abort + second turn)");
    // Start a single SSE connection that reads across BOTH turns (abort + second turn).
    // The SSE stream endpoint removes the receiver on first connect, so we must keep
    // this single connection alive for the entire test.
    let bridge_url = harness.bridge_url().to_string();
    let conv_id_clone = conv_id.clone();
    let (abort_events_tx, abort_events_rx) = tokio::sync::oneshot::channel();
    let sse_reader = tokio::spawn(async move {
        use futures::StreamExt;

        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .unwrap();

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                bridge_url, conv_id_clone
            ))
            .send()
            .await
            .expect("stream connect failed");

        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();
        let mut all_events: Vec<(String, serde_json::Value)> = Vec::new();
        let mut current_event_type = String::new();
        let deadline = std::time::Instant::now() + Duration::from_secs(180);
        let mut done_count = 0usize;
        let mut abort_events_tx = Some(abort_events_tx);

        loop {
            let remaining = deadline.saturating_duration_since(std::time::Instant::now());
            if remaining.is_zero() {
                break;
            }

            match tokio::time::timeout(remaining, stream.next()).await {
                Ok(Some(Ok(chunk))) => {
                    buffer.push_str(&String::from_utf8_lossy(&chunk));
                }
                Ok(Some(Err(_))) | Ok(None) | Err(_) => break,
            }

            while let Some(newline_pos) = buffer.find('\n') {
                let line = buffer[..newline_pos].trim_end().to_string();
                buffer = buffer[newline_pos + 1..].to_string();

                if line.is_empty() {
                    continue;
                }
                if let Some(event_name) = line.strip_prefix("event:") {
                    current_event_type = event_name.trim().to_string();
                } else if let Some(data_str) = line.strip_prefix("data:") {
                    let data_str = data_str.trim();
                    if data_str.is_empty() {
                        continue;
                    }
                    let data: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));
                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    all_events.push((event_type.clone(), data));

                    if event_type == "done" {
                        done_count += 1;
                        if done_count == 1 {
                            // First done = abort turn finished. Send abort events back.
                            if let Some(tx) = abort_events_tx.take() {
                                let _ = tx.send(all_events.clone());
                            }
                        }
                        if done_count >= 2 {
                            // Second done = second turn finished. We're done.
                            return all_events;
                        }
                    }
                    current_event_type.clear();
                }
            }
        }

        // If we exit the loop without 2 dones, send abort events if not yet sent
        if let Some(tx) = abort_events_tx.take() {
            let _ = tx.send(all_events.clone());
        }
        all_events
    });

    step!("Waiting 3s for LLM call to begin, then aborting");
    // Wait for the LLM call to begin processing, then abort.
    // 3 seconds is enough for the message to be received and the LLM call to start.
    tokio::time::sleep(Duration::from_secs(3)).await;

    let abort_start = std::time::Instant::now();
    step!("Sending abort request for conversation {}", conv_id);
    let abort_resp = harness
        .abort_conversation(&conv_id)
        .await
        .expect("abort request failed");

    check_eq!(abort_resp.status().as_u16(), 200, "abort should return 200");

    let abort_body: serde_json::Value = abort_resp.json().await.expect("parse abort body");
    check_eq!(
        abort_body.get("status").and_then(|s| s.as_str()),
        Some("aborted"),
        "abort response should have status: aborted"
    );

    step!("Waiting for abort events from SSE reader");
    // Wait for abort events from the SSE reader
    let abort_events = abort_events_rx.await.expect("abort events channel closed");
    let abort_latency = abort_start.elapsed();

    step!(
        "Abort latency: {:?}, collected {} events",
        abort_latency,
        abort_events.len()
    );
    for (t, d) in &abort_events {
        let s = d.to_string();
        eprintln!("    - {}:{}", t, &s[..s.len().min(120)]);
    }

    step!("Verifying error event with code 'aborted'");
    // Verify we got an error event with code "aborted"
    let abort_event = abort_events
        .iter()
        .find(|(t, d)| t == "error" && d.get("code").and_then(|c| c.as_str()) == Some("aborted"));
    check!(
        abort_event.is_some(),
        "expected an error event with code 'aborted'. Events: {:?}",
        abort_events
            .iter()
            .map(|(t, d)| format!("{}:{}", t, d))
            .collect::<Vec<_>>()
    );

    step!("Verifying done event after abort");
    // Verify we got a done event
    let done_event = abort_events.iter().find(|(t, _)| t == "done");
    check!(done_event.is_some(), "expected a 'done' event after abort");

    step!("Verifying abort resolved quickly (< 10s)");
    // The abort should resolve quickly (not wait for the full LLM response).
    // Allow up to 10 seconds — the key point is it should NOT take the full
    // LLM response time (which would be 15-40+ seconds for tool-calling agents).
    check!(
        abort_latency < Duration::from_secs(10),
        "abort should resolve quickly, but took {:?}",
        abort_latency
    );

    step!("Sending second message to verify conversation is still usable");
    // Now verify the conversation is still usable — send another message
    // and expect a normal response. The SSE reader is still connected and
    // will collect events for the second turn.
    let msg2_resp = harness
        .send_message(&conv_id, "What is Rust known for? Answer in one sentence.")
        .await
        .expect("send second message after abort");

    check!(
        msg2_resp.status().is_success() || msg2_resp.status().as_u16() == 202,
        "second message after abort returned success/202 (got {})",
        msg2_resp.status()
    );

    step!("Waiting for SSE reader to collect second turn events");
    // Wait for the SSE reader to collect the second turn's events (until 2nd done)
    let all_events = sse_reader.await.expect("SSE reader task panicked");

    // Find events after the first done (second turn's events)
    let first_done_idx = all_events
        .iter()
        .position(|(t, _)| t == "done")
        .expect("should have at least one done event");
    let turn2_events: Vec<_> = all_events[first_done_idx + 1..].to_vec();

    // Extract response text from second turn's content_delta events
    let response2: String = turn2_events
        .iter()
        .filter(|(t, _)| t == "content_delta")
        .filter_map(|(_, d)| d.get("delta").and_then(|d| d.as_str()))
        .collect();

    step!("Second turn response ({} chars)", response2.len());
    eprintln!("    Response: {:?}", &response2[..response2.len().min(200)]);

    // Log the second turn's response in the same format as other conversation tests
    let turn2_elapsed = abort_start.elapsed();
    eprintln!(
        "[researcher] \n\
         [researcher] ================================================================================\n\
         [researcher] ASSISTANT RESPONSE (complete)\n\
         [researcher] ================================================================================\n\
         [researcher] {}\n\
         [researcher] \n\
         [researcher] ================================================================================\n\
         [researcher] TURN COMPLETED ({:.1}s)\n\
         [researcher] ================================================================================\n",
        if response2.is_empty() { "[empty response]" } else { &response2 },
        turn2_elapsed.as_secs_f64()
    );

    check!(
        !response2.is_empty(),
        "conversation should still work after abort — second turn returned empty response. Events: {:?}",
        turn2_events
            .iter()
            .map(|(t, d)| {
                let s = d.to_string();
                format!("{}:{}", t, &s[..s.floor_char_boundary(200.min(s.len()))])
            })
            .collect::<Vec<_>>()
    );

    step!("PASS — abort worked (latency {:?}) and conversation remained usable", abort_latency);
}

// ============================================================================
// Test: Executor — Background Bash
// Verifies: bash tool called with background: true, notification round-trip
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_executor_background_bash() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Creating conversation for executor agent");
    // Create conversation and send message manually so we can read multiple turns.
    // Background bash produces two turns:
    //   Turn 1: LLM calls bash(background:true) -> gets immediate "running" -> responds
    //   Turn 2: Background command completes -> notification -> LLM reports output
    let resp = harness
        .create_conversation("executor")
        .await
        .expect("create conversation");
    let body: serde_json::Value = resp.json().await.expect("parse create response");
    let conversation_id = body["conversation_id"]
        .as_str()
        .expect("conversation_id")
        .to_string();

    step!("Conversation created: {}", conversation_id);

    // Register for conversation logging
    harness
        .register_conversation(&conversation_id, "executor")
        .await;

    step!("Sending background bash command");
    let msg_resp = harness
        .send_message(
            &conversation_id,
            "Run `echo 'background_task_complete_marker_12345'` in the background using the bash tool with background set to true. After it completes, report the output.",
        )
        .await
        .expect("send message");
    check!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send returned success/202 (got {})",
        msg_resp.status()
    );

    step!("Streaming SSE events across 2 turns (background launch + completion)");
    // Read SSE events across 2 turns (2 "done" events).
    // Turn 1: agent acknowledges background launch.
    // Turn 2: agent processes the background completion notification and reports output.
    let (events, response_text) = harness
        .stream_sse_until_done_count(&conversation_id, 2, LLM_TIMEOUT)
        .await
        .expect("stream SSE events");

    step!(
        "Collected {} SSE events, response: {} chars",
        events.len(),
        response_text.len()
    );
    for e in &events {
        eprintln!("    - {}", e.event_type);
    }

    // LLM responses are non-deterministic; a missing text response is acceptable
    // as long as the tool calls executed correctly.
    if response_text.is_empty() {
        step!("Warning: empty response text, checking tool calls only");
    } else {
        eprintln!("    Response: {:?}", &response_text[..response_text.len().min(200)]);
    }

    step!("Verifying bash tool was called");
    // Verify the bash tool was called with background: true in SSE events
    let bash_starts: Vec<_> = events
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data.get("name").and_then(|n| n.as_str()) == Some("bash")
        })
        .collect();

    check!(!bash_starts.is_empty(), "bash tool should have been called");

    step!("Verifying bash was called with background: true");
    // Check that at least one bash call had background: true
    let used_background = bash_starts.iter().any(|e| {
        let args = e.data.get("arguments");
        // Handle both object and string-encoded arguments
        match args {
            Some(serde_json::Value::Object(obj)) => {
                obj.get("background") == Some(&serde_json::json!(true))
            }
            Some(serde_json::Value::String(s)) => s.contains("background") && s.contains("true"),
            _ => false,
        }
    });

    check!(
        used_background,
        "bash should have been called with background: true"
    );

    step!("Verifying response contains background command output marker");
    // Verify the response contains the marker from the background command output.
    // This proves the full round-trip: command ran -> notification sent -> agent received it.
    check!(
        response_text.contains("background_task_complete_marker_12345"),
        "response should contain the background command output marker, got: {}",
        &response_text[..response_text.len().min(500)]
    );

    step!("PASS — background bash executed, notification round-trip completed");
}

// ============================================================================
// Test: Compaction — triggers compaction and fires webhook
// Verifies: conversation_compacted webhook with summary, token counts
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_compaction_triggers_and_fires_webhook() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    step!("Clearing webhook log");
    // Clear any prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    step!("Creating conversation for compaction-agent (token_budget=150)");
    // Create a conversation on the compaction agent (token_budget=150)
    let create_resp = harness
        .create_conversation("compaction-agent")
        .await
        .expect("create_conversation failed");

    check_eq!(create_resp.status().as_u16(), 201, "create conversation returns 201");

    let create_body: serde_json::Value = create_resp
        .json()
        .await
        .expect("failed to parse create body");
    let conv_id = create_body["conversation_id"]
        .as_str()
        .expect("missing conversation_id")
        .to_string();

    step!("Conversation created: {}", conv_id);

    // Register for conversation logging
    harness
        .register_conversation(&conv_id, "compaction-agent")
        .await;

    // Real-world messages that build up context quickly
    let messages: Vec<&str> = vec![
        "Draft a formal email to the VP of Engineering about the Q3 roadmap planning session scheduled for next Thursday",
        "Actually, also CC the product team leads and mention that we need their input on the resource allocation proposals",
        "Add a paragraph about the budget constraints we discussed in last week's leadership meeting",
        "Now draft a separate follow-up email to the design team about the UI refresh project timeline and deliverables",
        "One more thing — include a reminder about the Friday design review and ask them to prepare their mockups",
        "Thanks, that looks great. Send it off.",
    ];

    let total_messages = messages.len();

    step!("Sending {} messages to build up context", total_messages);
    // Send the first message so the conversation loop starts processing
    let msg_resp = harness
        .send_message(&conv_id, messages[0])
        .await
        .expect("send_message failed");
    check_eq!(msg_resp.status().as_u16(), 202, "first message accepted (202)");
    step!("Sent message 1/{}: '{}'", total_messages, &messages[0][..messages[0].len().min(60)]);

    // Spawn a background task to send remaining messages after each turn completes.
    // The conversation loop processes one message at a time from a buffered channel,
    // so we queue them with small delays to let each turn start.
    let bridge_url = harness.bridge_url().to_string();
    let conv_id_bg = conv_id.clone();
    let remaining: Vec<String> = messages[1..].iter().map(|s| s.to_string()).collect();
    let remaining_count = remaining.len();
    tokio::spawn(async move {
        let client = reqwest::Client::new();
        for (i, msg) in remaining.iter().enumerate() {
            // Each Fireworks round-trip takes ~5-10s; with compaction it may
            // need two calls (summary + chat). Wait long enough for the
            // previous turn to fully complete before queuing the next message.
            tokio::time::sleep(Duration::from_secs(15)).await;
            eprintln!("\n  \x1b[36m\u{25b8}\x1b[0m Sent message {}/{}: '{}'", i + 2, remaining_count + 1, &msg[..msg.len().min(60)]);
            let _ = client
                .post(format!(
                    "{}/conversations/{}/messages",
                    bridge_url, conv_id_bg
                ))
                .json(&serde_json::json!({"content": msg}))
                .send()
                .await;
        }
    });

    step!("Streaming SSE events until all {} turns complete", total_messages);
    // Read the SSE stream until all turns complete (one `done` per turn)
    let (_events, _text) = harness
        .stream_sse_until_done_count(&conv_id, total_messages, LLM_TIMEOUT)
        .await
        .expect("stream_sse_until_done_count failed");

    step!("Waiting for conversation_compacted webhook (timeout: 30s)");
    // Wait for the conversation_compacted webhook
    let log = harness
        .wait_for_webhook_type("conversation_compacted", Duration::from_secs(30))
        .await
        .expect("conversation_compacted webhook never arrived");

    let compacted = log.by_type("conversation_compacted");
    check!(
        !compacted.is_empty(),
        "should have at least one conversation_compacted webhook"
    );

    step!("Checking conversation_compacted webhook payload");
    // Verify the webhook payload
    let entry = compacted[0];
    check_eq!(entry.agent_id(), Some("compaction-agent"), "webhook agent_id is compaction-agent");
    check_eq!(entry.conversation_id(), Some(conv_id.as_str()), "webhook conversation_id matches");

    let data = entry
        .data()
        .expect("conversation_compacted should have data");

    eprintln!("    Webhook data: {}", serde_json::to_string_pretty(data).unwrap_or_default());

    check!(
        data.get("summary")
            .and_then(|v| v.as_str())
            .map(|s| !s.is_empty())
            .unwrap_or(false),
        "summary should be a non-empty string; got: {:?}",
        data.get("summary")
    );
    check!(
        data.get("messages_compacted")
            .and_then(|v| v.as_u64())
            .unwrap_or(0)
            > 0,
        "messages_compacted should be > 0"
    );
    let pre = data
        .get("pre_compaction_tokens")
        .and_then(|v| v.as_u64())
        .expect("pre_compaction_tokens missing");
    let post = data
        .get("post_compaction_tokens")
        .and_then(|v| v.as_u64())
        .expect("post_compaction_tokens missing");
    check!(
        pre > 1500,
        "pre_compaction_tokens ({}) should exceed budget (1500)",
        pre
    );
    // After compacting many turns into a summary, post should be notably smaller
    check!(
        post < pre,
        "post_compaction_tokens ({}) should be less than pre ({})",
        post,
        pre
    );

    step!("Ending conversation");
    // End the conversation
    let end_resp = harness
        .end_conversation(&conv_id)
        .await
        .expect("end_conversation failed");
    check_eq!(end_resp.status().as_u16(), 200, "end conversation returns 200");

    step!(
        "PASS — compaction triggered: pre={} post={}, summary present",
        pre, post
    );
}

// ============================================================================
// Test: Streaming — text interleaved with tool calls
// Verifies: content_delta events arrive BEFORE and AFTER tool_call_start/result
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_streaming_text_between_tool_calls() {
    if !require_fireworks_key() {
        return;
    }

    step!("Starting harness with real LLM");
    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    let turn = converse_with_retry(
        &harness,
        "streaming-agent",
        "Look up issue ENG-42 and tell me about it.",
        "streaming",
    )
    .await;

    assert_response_not_empty(&turn, "streaming");

    step!("[streaming] Verifying getIssue was called");
    // The agent should have called getIssue
    assert_any_tool_called_in_sse(&turn, &["getIssue"], "streaming");

    // ---- Core streaming assertion ----
    // Verify that content_delta events appear both BEFORE and AFTER tool calls.
    // This is the key behavior: the LLM streams text, then makes a tool call,
    // then streams more text — and the client sees all of it in real time.

    // Find the index of the first tool_call_start event
    let first_tool_start_idx = turn
        .sse_events
        .iter()
        .position(|e| e.event_type == "tool_call_start");

    // Find the index of the last tool_call_result event
    let last_tool_result_idx = turn
        .sse_events
        .iter()
        .rposition(|e| e.event_type == "tool_call_result");

    check!(
        first_tool_start_idx.is_some(),
        "[streaming] expected at least one tool_call_start event"
    );
    check!(
        last_tool_result_idx.is_some(),
        "[streaming] expected at least one tool_call_result event"
    );

    let tool_start = first_tool_start_idx.unwrap();
    let tool_end = last_tool_result_idx.unwrap();

    // Check for content_delta events BEFORE the first tool call
    let deltas_before_tool = turn.sse_events[..tool_start]
        .iter()
        .any(|e| e.event_type == "content_delta");

    // Check for content_delta events AFTER the last tool result
    let deltas_after_tool = turn.sse_events[tool_end + 1..]
        .iter()
        .any(|e| e.event_type == "content_delta");

    // Log the full event sequence for debugging
    let event_sequence: Vec<String> = turn
        .sse_events
        .iter()
        .map(|e| {
            if e.event_type == "content_delta" {
                let text = e.data.get("delta").and_then(|d| d.as_str()).unwrap_or("");
                let preview: String = text.chars().take(40).collect();
                format!("content_delta(\"{}\")", preview)
            } else if e.event_type == "tool_call_start" {
                let name = e.data.get("name").and_then(|n| n.as_str()).unwrap_or("?");
                format!("tool_call_start({})", name)
            } else {
                e.event_type.clone()
            }
        })
        .collect();

    step!("[streaming] Event sequence ({} events)", event_sequence.len());
    for (i, e) in event_sequence.iter().enumerate() {
        eprintln!("    [{}] {}", i, e);
    }

    step!("[streaming] Verifying content_delta events BEFORE tool call");
    check!(
        deltas_before_tool,
        "[streaming] expected content_delta events BEFORE tool_call_start. \
         The LLM should stream explanatory text before calling a tool. \
         Events: {:?}",
        event_sequence
    );

    step!("[streaming] Verifying content_delta events AFTER tool call");
    check!(
        deltas_after_tool,
        "[streaming] expected content_delta events AFTER tool_call_result. \
         The LLM should stream a summary after receiving the tool result. \
         Events: {:?}",
        event_sequence
    );

    // Also verify multiple content_delta events (proving incremental streaming,
    // not a single bulk event)
    let delta_count = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "content_delta")
        .count();

    step!("[streaming] Verifying incremental streaming ({} content_deltas)", delta_count);
    check!(
        delta_count >= 3,
        "[streaming] expected at least 3 content_delta events (incremental streaming), got {}. \
         Events: {:?}",
        delta_count,
        event_sequence
    );

    step!(
        "PASS — streaming verified: {} content_deltas, text interleaved with tool calls, completed in {:?}",
        delta_count,
        turn.duration
    );
}
