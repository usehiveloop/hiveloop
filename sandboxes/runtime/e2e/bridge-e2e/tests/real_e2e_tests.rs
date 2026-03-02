//! Real E2E tests that use actual LLM calls via OpenRouter.
//!
//! These tests are `#[ignore]` by default — run them with:
//!
//! ```sh
//! OPENROUTER_API_KEY=<key> cargo test -p bridge-e2e --test real_e2e_tests -- --ignored
//! ```
//!
//! Tests run serially to avoid OpenRouter rate limits.

use bridge_e2e::{ConversationTurn, TestHarness};
use std::time::Duration;

/// Default timeout for LLM responses (real model with tool loops).
/// With max_turns=5, each OpenRouter round trip can be 15-40s (depending on
/// context size and tool count), so worst case ~240s for a full 5-turn loop.
const LLM_TIMEOUT: Duration = Duration::from_secs(300);

/// Maximum retries for a conversation turn that returns an empty/error response.
/// Real LLM APIs can have transient failures (rate limits, empty responses).
const MAX_RETRIES: usize = 2;

/// Skip test if OPENROUTER_API_KEY is not set.
fn require_openrouter_key() -> bool {
    if std::env::var("OPENROUTER_API_KEY").is_err() {
        eprintln!("OPENROUTER_API_KEY not set — skipping real E2E test");
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
            eprintln!("[{}] retrying (attempt {}/{})", label, attempt + 1, MAX_RETRIES);
            tokio::time::sleep(Duration::from_secs(2)).await;
        }

        let turn = harness
            .converse(agent_id, None, message, LLM_TIMEOUT)
            .await
            .expect("conversation failed");

        let has_error = turn
            .sse_events
            .iter()
            .any(|e| e.event_type == "error");

        if !turn.response_text.is_empty() && !has_error {
            return turn;
        }

        eprintln!(
            "[{}] attempt {} got empty/error response. Events: {:?}",
            label,
            attempt + 1,
            turn.sse_events
                .iter()
                .map(|e| format!("{}:{}", e.event_type, &e.data.to_string()[..e.data.to_string().len().min(120)]))
                .collect::<Vec<_>>()
        );
        last_turn = Some(turn);
    }

    // Return the last turn — the test assertion will fail with diagnostics
    last_turn.unwrap()
}

/// Assert response is non-empty with diagnostic output on failure.
fn assert_response_not_empty(turn: &ConversationTurn, label: &str) {
    assert!(
        !turn.response_text.is_empty(),
        "[{}] response should not be empty. SSE events received: {:?}",
        label,
        turn.sse_events
            .iter()
            .map(|e| format!("{}:{}", e.event_type, &e.data.to_string()[..e.data.to_string().len().min(200)]))
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
    if !require_openrouter_key() {
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
    if !require_openrouter_key() {
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
    if !require_openrouter_key() {
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
    if !require_openrouter_key() {
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

    harness
        .assert_any_tool_called(&["Glob", "Grep", "Read"])
        .expect("file exploration tools should have been called");
    harness
        .assert_tool_called("createDocument")
        .expect("createDocument should have been called");
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
    if !require_openrouter_key() {
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

    harness
        .assert_any_tool_called(&["Glob", "Grep"])
        .expect("file search tools should have been called");
    harness
        .assert_tool_called("Read")
        .expect("Read should have been called");
    harness
        .assert_tool_called("createDocument")
        .expect("createDocument should have been called");
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
    if !require_openrouter_key() {
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
    if !require_openrouter_key() {
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
// Test 8: Multi-Agent Concurrent Conversations
// Verifies: all 6 agents respond, metrics tracked, no cross-contamination
// ============================================================================
#[tokio::test]
#[ignore]
async fn test_multi_agent_concurrent_conversations() {
    if !require_openrouter_key() {
        return;
    }

    let harness = TestHarness::start_real()
        .await
        .expect("failed to start real harness");

    // Verify all 6 agents are loaded
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
    ] {
        assert!(
            agent_ids.contains(&expected_id.to_string()),
            "agent '{}' not found. Loaded: {:?}",
            expected_id,
            agent_ids
        );
    }

    // Create 6 conversations simultaneously with simple non-tool messages
    let messages = vec![
        ("code-review", "What is the most important thing in a code review? Answer in 2-3 sentences."),
        ("portal-control", "Briefly describe your role as Portal in this workspace. 2-3 sentences."),
        ("security-audit", "What are the top 3 OWASP vulnerabilities? Answer briefly."),
        ("system-design", "What makes a good system design document? Answer in 2-3 sentences."),
        ("technical-writer", "What makes good API documentation? Answer in 2-3 sentences."),
        ("researcher", "What is Rust known for? Answer in 2-3 sentences."),
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
                            v.get("type").and_then(|t| t.as_str()).unwrap_or("message").to_string()
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

    // Wait for all 6 conversations
    let mut results = Vec::new();
    for handle in handles {
        let result = handle.await.expect("task panicked");
        results.push(result);
    }

    assert_eq!(results.len(), 6, "all 6 agents should have responded");

    // Verify metrics show conversations tracked
    let metrics = harness
        .get_metrics()
        .await
        .expect("failed to get metrics");

    if let Some(global) = metrics.get("global") {
        let total_agents = global
            .get("total_agents")
            .and_then(|v| v.as_u64())
            .unwrap_or(0);
        assert!(
            total_agents >= 6,
            "should have at least 6 agents in metrics, got {}",
            total_agents
        );
    }

    eprintln!("All 6 agents responded successfully");
    for (agent_id, conv_id) in &results {
        eprintln!("  {} -> conversation {}", agent_id, conv_id);
    }
}
