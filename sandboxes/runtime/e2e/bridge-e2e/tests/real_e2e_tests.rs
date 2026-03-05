//! Real E2E tests that use actual LLM calls via Fireworks.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! FIREWORKS_API_KEY=<key> cargo test -p bridge-e2e --test real_e2e_tests -- --ignored
//! ```
//!
//! Tests run serially to avoid Fireworks rate limits.

use bridge_e2e::{ConversationTurn, TestHarness};
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
            eprintln!(
                "[{}] retrying (attempt {}/{})",
                label,
                attempt + 1,
                MAX_RETRIES
            );
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn.sse_events.iter().any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            return turn;
        }

        eprintln!(
            "[{}] attempt {} got empty/error response. Events: {:?}",
            label,
            attempt + 1,
            turn.sse_events
                .iter()
                .map(|e| format!(
                    "{}:{}",
                    e.event_type,
                    &e.data.to_string()[..e.data.to_string().len().min(120)]
                ))
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
    assert!(
        found,
        "[{}] none of {:?} were called. Tools called (from SSE): {:?}",
        label, tool_names, called_tools
    );
}

/// Assert response is non-empty with diagnostic output on failure.
fn assert_response_not_empty(turn: &ConversationTurn, label: &str) {
    assert!(
        !turn.response_text.is_empty(),
        "[{}] response should not be empty. SSE events received: {:?}",
        label,
        turn.sse_events
            .iter()
            .map(|e| format!(
                "{}:{}",
                e.event_type,
                &e.data.to_string()[..e.data.to_string().len().min(200)]
            ))
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

    harness
        .assert_tool_called("getIssue")
        .expect("getIssue should have been called");
    harness
        .assert_any_tool_called(&["listIssuePullRequests", "getPullRequest"])
        .expect("PR-related tools should have been called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    eprintln!(
        "[hana] completed in {:?}, response: {} chars",
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Turn 1: List issues
    let turn1 = converse_with_retry(
        &harness,
        "portal-control",
        "What issues does the ENG team currently have? Give me a brief status summary.",
        "nova turn1",
    )
    .await;

    assert_response_not_empty(&turn1, "nova turn1");

    harness
        .assert_any_tool_called(&["listTeamIssues", "listTeams", "getTeam"])
        .expect("team/issue tools should have been called");

    eprintln!(
        "[nova turn1] completed in {:?}, response: {} chars",
        turn1.duration,
        turn1.response_text.len()
    );

    // Clear log for turn 2 assertions
    let log_dir = std::env::temp_dir().join("portal-mcp-logs");
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 2: Create issue (separate conversation)
    let turn2 = converse_with_retry(
        &harness,
        "portal-control",
        "Create a new high-priority issue on the ENG team titled 'Implement API rate limiting'. You have my approval — go ahead and use createIssue directly.",
        "nova turn2",
    )
    .await;

    assert_response_not_empty(&turn2, "nova turn2");

    harness
        .assert_any_tool_called(&["createIssue", "submitApprovalRequest"])
        .expect("issue creation or approval tools should have been called");

    eprintln!(
        "[nova turn2] completed in {:?}, response: {} chars",
        turn2.duration,
        turn2.response_text.len()
    );

    // Clear log for turn 3
    let _ = std::fs::remove_dir_all(&log_dir);
    let _ = std::fs::create_dir_all(&log_dir);

    // Turn 3: Ping human (separate conversation)
    let turn3 = converse_with_retry(
        &harness,
        "portal-control",
        "Use the pingHuman tool to alert the team lead that we need human review on the rate limiting approach. It's urgent.",
        "nova turn3",
    )
    .await;

    assert_response_not_empty(&turn3, "nova turn3");

    harness
        .assert_tool_called("pingHuman")
        .expect("pingHuman should have been called");

    eprintln!(
        "[nova turn3] completed in {:?}, response: {} chars",
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

    harness
        .assert_tool_called("getIssue")
        .expect("getIssue should have been called");
    harness
        .assert_any_tool_called(&["listIssuePullRequests", "getPullRequest"])
        .expect("PR tools should have been called");
    harness
        .assert_any_tool_called(&["createComment", "addPullRequestComment"])
        .expect("comment tools should have been called");

    eprintln!(
        "[skai] completed in {:?}, response: {} chars",
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
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    eprintln!(
        "[theo] completed in {:?}, response: {} chars",
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
    harness
        .assert_any_tool_called(&["createDocument", "updateDocument"])
        .expect("document creation tools should have been called");
    harness
        .assert_tool_called("createComment")
        .expect("createComment should have been called");

    eprintln!(
        "[mimi] completed in {:?}, response: {} chars",
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

    // The mock search endpoint returns results with unique markers and
    // content about Tokio, async/await, Futures. The agent MUST use the
    // web_search tool to know these — it can't fabricate the markers.
    let has_search_content = response.contains("tokio")
        || response.contains("async")
        || response.contains("await")
        || response.contains("bridge_e2e_search_marker");

    assert!(
        has_search_content,
        "response should contain content from search results. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    eprintln!(
        "[researcher] completed in {:?}, response: {} chars",
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

    // The agent must have actually fetched rust-lang.org to know this content
    let has_fetch_content = response.contains("rust")
        && (response.contains("performance")
            || response.contains("reliable")
            || response.contains("memory")
            || response.contains("safety")
            || response.contains("concurrency"));

    assert!(
        has_fetch_content,
        "response should contain content from rust-lang.org. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    eprintln!(
        "[researcher-fetch] completed in {:?}, response: {} chars",
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

    eprintln!(
        "[tool-call-sse] tool_call_start events: {}, tool_call_result events: {}",
        start_events.len(),
        result_events.len()
    );

    // Assert tool_call_start events exist
    assert!(
        !start_events.is_empty(),
        "expected at least one tool_call_start SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    // Assert tool_call_result events exist
    assert!(
        !result_events.is_empty(),
        "expected at least one tool_call_result SSE event. All events: {:?}",
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    // Each start should have a matching result (same count)
    assert_eq!(
        start_events.len(),
        result_events.len(),
        "tool_call_start count ({}) should match tool_call_result count ({})",
        start_events.len(),
        result_events.len()
    );

    // Validate tool_call_start data contains required fields
    for event in &start_events {
        assert!(
            event.data.get("name").is_some(),
            "tool_call_start should have 'name' field: {:?}",
            event.data
        );
        assert!(
            event.data.get("arguments").is_some(),
            "tool_call_start should have 'arguments' field: {:?}",
            event.data
        );
        assert!(
            event.data.get("id").is_some(),
            "tool_call_start should have 'id' field: {:?}",
            event.data
        );
    }

    // Validate tool_call_result data contains required fields
    for event in &result_events {
        assert!(
            event.data.get("result").is_some(),
            "tool_call_result should have 'result' field: {:?}",
            event.data
        );
        assert!(
            event.data.get("is_error").is_some(),
            "tool_call_result should have 'is_error' field: {:?}",
            event.data
        );
        assert!(
            event.data.get("id").is_some(),
            "tool_call_result should have 'id' field: {:?}",
            event.data
        );
    }

    eprintln!(
        "[tool-call-sse] completed in {:?}, response: {} chars, {} tool calls",
        turn.duration,
        turn.response_text.len(),
        start_events.len()
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Verify the delegator agent is loaded
    let agents = harness.get_agents().await.expect("failed to get agents");
    let agent_ids: Vec<String> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()).map(String::from))
        .collect();

    assert!(
        agent_ids.contains(&"delegator".to_string()),
        "delegator agent should be loaded. Loaded agents: {:?}",
        agent_ids
    );

    // Send a natural user message that requires codebase exploration.
    // The system prompt tells the agent to delegate to the 'explorer' subagent
    // for file-related tasks, but the user message does NOT mention subagents.
    let turn = converse_with_retry(
        &harness,
        "delegator",
        "What is the structure of this project? List the main source directories and describe what each crate does based on the files you find.",
        "delegator-natural",
    )
    .await;

    assert_response_not_empty(&turn, "delegator-natural");

    // Verify the agent tool was invoked by checking SSE events.
    // The LLM should have decided to use the agent tool based on the system prompt.
    let agent_tool_starts: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data.get("name").and_then(|n| n.as_str()) == Some("agent")
        })
        .collect();

    assert!(
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
                &e.data.to_string()[..e.data.to_string().len().min(200)]
            ))
            .collect::<Vec<_>>(),
        turn.sse_events
            .iter()
            .map(|e| &e.event_type)
            .collect::<Vec<_>>()
    );

    // Verify the subagent was 'explorer' (the system prompt directs file exploration there)
    let used_explorer = agent_tool_starts.iter().any(|e| {
        e.data
            .get("arguments")
            .and_then(|a| a.get("subagent"))
            .and_then(|s| s.as_str())
            == Some("explorer")
    });

    // Also accept string-encoded arguments (some providers send args as a JSON string)
    let used_explorer = used_explorer
        || agent_tool_starts.iter().any(|e| {
            e.data
                .get("arguments")
                .and_then(|a| a.as_str())
                .map(|s| s.contains("explorer"))
                .unwrap_or(false)
        });

    assert!(
        used_explorer,
        "[delegator-natural] expected 'explorer' subagent to be invoked for a codebase \
         exploration task. Agent tool calls: {:?}",
        agent_tool_starts
            .iter()
            .map(|e| e.data.to_string())
            .collect::<Vec<_>>()
    );

    // Verify there's a corresponding tool_call_result
    let agent_tool_results: Vec<_> = turn
        .sse_events
        .iter()
        .filter(|e| e.event_type == "tool_call_result")
        .collect();

    assert!(
        !agent_tool_results.is_empty(),
        "[delegator-natural] expected tool_call_result events after agent invocation"
    );

    // The response should contain information about the project structure
    // (since the explorer subagent has file tools and should have found files)
    let response_lower = turn.response_text.to_lowercase();
    let has_structure_info = response_lower.contains("crate")
        || response_lower.contains("src")
        || response_lower.contains("directory")
        || response_lower.contains("module")
        || response_lower.contains("file")
        || response_lower.contains("project");

    assert!(
        has_structure_info,
        "[delegator-natural] response should contain project structure information. Got: {}",
        &turn.response_text[..turn.response_text.len().min(500)]
    );

    eprintln!(
        "[delegator-natural] completed in {:?}, response: {} chars, agent tool calls: {}",
        turn.duration,
        turn.response_text.len(),
        agent_tool_starts.len()
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

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
        assert!(
            agent_ids.contains(&expected_id.to_string()),
            "agent '{}' not found. Loaded: {:?}",
            expected_id,
            agent_ids
        );
    }

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
                "[concurrent] {} responded ({} chars)",
                agent_id,
                response_text.len()
            );

            (agent_id, conv_id)
        }));
    }

    // Wait for all 8 conversations
    let mut results = Vec::new();
    for handle in handles {
        let result = handle.await.expect("task panicked");
        results.push(result);
    }

    assert_eq!(results.len(), 8, "all 8 agents should have responded");

    // Verify metrics show conversations tracked
    let metrics = harness.get_metrics().await.expect("failed to get metrics");

    if let Some(global) = metrics.get("global") {
        let total_agents = global
            .get("total_agents")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        assert!(
            total_agents >= 8,
            "should have at least 8 agents in metrics, got {}",
            total_agents
        );
    }

    eprintln!("All 8 agents responded successfully");
    for (agent_id, conv_id) in &results {
        eprintln!("  {} -> conversation {}", agent_id, conv_id);
    }
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

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

    harness.register_conversation(&conv_id, "researcher").await;

    // Send a message that will trigger tool calls (web_search takes time)
    let msg_resp = harness
        .send_message(
            &conv_id,
            "Research the history of the Rust programming language in depth. Use the web_search tool to search for 'Rust programming language history timeline'. Then search for 'Rust borrow checker design'. Then search for 'Rust async await RFC history'. Give me a comprehensive report.",
        )
        .await
        .expect("send message");
    assert!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send failed: {}",
        msg_resp.status()
    );

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

    // Wait for the LLM call to begin processing, then abort.
    // 3 seconds is enough for the message to be received and the LLM call to start.
    tokio::time::sleep(Duration::from_secs(3)).await;

    let abort_start = std::time::Instant::now();
    let abort_resp = harness
        .abort_conversation(&conv_id)
        .await
        .expect("abort request failed");

    assert_eq!(abort_resp.status().as_u16(), 200, "abort should return 200");

    let abort_body: serde_json::Value = abort_resp.json().await.expect("parse abort body");
    assert_eq!(
        abort_body.get("status").and_then(|s| s.as_str()),
        Some("aborted"),
        "abort response should have status: aborted"
    );

    // Wait for abort events from the SSE reader
    let abort_events = abort_events_rx.await.expect("abort events channel closed");
    let abort_latency = abort_start.elapsed();

    eprintln!(
        "[abort] abort latency: {:?}, collected {} events: {:?}",
        abort_latency,
        abort_events.len(),
        abort_events
            .iter()
            .map(|(t, d)| format!("{}:{}", t, &d.to_string()[..d.to_string().len().min(120)]))
            .collect::<Vec<_>>()
    );

    // Verify we got an error event with code "aborted"
    let abort_event = abort_events
        .iter()
        .find(|(t, d)| t == "error" && d.get("code").and_then(|c| c.as_str()) == Some("aborted"));
    assert!(
        abort_event.is_some(),
        "expected an error event with code 'aborted'. Events: {:?}",
        abort_events
            .iter()
            .map(|(t, d)| format!("{}:{}", t, d))
            .collect::<Vec<_>>()
    );

    // Verify we got a done event
    let done_event = abort_events.iter().find(|(t, _)| t == "done");
    assert!(done_event.is_some(), "expected a 'done' event after abort");

    // The abort should resolve quickly (not wait for the full LLM response).
    // Allow up to 10 seconds — the key point is it should NOT take the full
    // LLM response time (which would be 15-40+ seconds for tool-calling agents).
    assert!(
        abort_latency < Duration::from_secs(10),
        "abort should resolve quickly, but took {:?}",
        abort_latency
    );

    // Now verify the conversation is still usable — send another message
    // and expect a normal response. The SSE reader is still connected and
    // will collect events for the second turn.
    let msg2_resp = harness
        .send_message(&conv_id, "What is Rust known for? Answer in one sentence.")
        .await
        .expect("send second message after abort");

    assert!(
        msg2_resp.status().is_success() || msg2_resp.status().as_u16() == 202,
        "second message after abort failed: {}",
        msg2_resp.status()
    );

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

    assert!(
        !response2.is_empty(),
        "conversation should still work after abort — second turn returned empty response. Events: {:?}",
        turn2_events
            .iter()
            .map(|(t, d)| format!("{}:{}", t, &d.to_string()[..d.to_string().len().min(200)]))
            .collect::<Vec<_>>()
    );

    eprintln!("[abort] test passed — abort worked and conversation remained usable");
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

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Create conversation and send message manually so we can read multiple turns.
    // Background bash produces two turns:
    //   Turn 1: LLM calls bash(background:true) → gets immediate "running" → responds
    //   Turn 2: Background command completes → notification → LLM reports output
    let resp = harness
        .create_conversation("executor")
        .await
        .expect("create conversation");
    let body: serde_json::Value = resp.json().await.expect("parse create response");
    let conversation_id = body["conversation_id"]
        .as_str()
        .expect("conversation_id")
        .to_string();

    // Register for conversation logging
    harness
        .register_conversation(&conversation_id, "executor")
        .await;

    let msg_resp = harness
        .send_message(
            &conversation_id,
            "Run `echo 'background_task_complete_marker_12345'` in the background using the bash tool with background set to true. After it completes, report the output.",
        )
        .await
        .expect("send message");
    assert!(
        msg_resp.status().is_success() || msg_resp.status().as_u16() == 202,
        "message send failed: {}",
        msg_resp.status()
    );

    // Read SSE events across 2 turns (2 "done" events).
    // Turn 1: agent acknowledges background launch.
    // Turn 2: agent processes the background completion notification and reports output.
    let (events, response_text) = harness
        .stream_sse_until_done_count(&conversation_id, 2, LLM_TIMEOUT)
        .await
        .expect("stream SSE events");

    eprintln!(
        "[executor-bg] collected {} events, response: {} chars",
        events.len(),
        response_text.len()
    );

    assert!(
        !response_text.is_empty(),
        "[executor-bg] response should not be empty"
    );

    // Verify the bash tool was called with background: true in SSE events
    let bash_starts: Vec<_> = events
        .iter()
        .filter(|e| {
            e.event_type == "tool_call_start"
                && e.data.get("name").and_then(|n| n.as_str()) == Some("bash")
        })
        .collect();

    assert!(!bash_starts.is_empty(), "bash tool should have been called");

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

    assert!(
        used_background,
        "bash should have been called with background: true"
    );

    // Verify the response contains the marker from the background command output.
    // This proves the full round-trip: command ran → notification sent → agent received it.
    assert!(
        response_text.contains("background_task_complete_marker_12345"),
        "response should contain the background command output marker, got: {}",
        &response_text[..response_text.len().min(500)]
    );
}
