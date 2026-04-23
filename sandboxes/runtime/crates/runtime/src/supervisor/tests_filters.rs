use super::helpers::filter_conversation_tools;
use super::tests_mock::make_tools;
use std::collections::HashMap;

// ── filter_conversation_tools: no filters ─────────────────────────────────

#[test]
fn no_filters_returns_all_tools() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "write", "glob"]);
    let mcp_map = HashMap::new();

    let result =
        filter_conversation_tools("agent1", &mut names, &mut executors, &mcp_map, None, None);

    assert!(result.is_ok());
    assert_eq!(names.len(), 4);
    assert_eq!(executors.len(), 4);
}

// ── filter_conversation_tools: tool_names filter ──────────────────────────

#[test]
fn tool_names_filter_retains_only_requested() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "write", "glob"]);
    let mcp_map = HashMap::new();
    let filter = vec!["bash".to_string(), "read".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        None,
        Some(&filter),
    );

    assert!(result.is_ok());
    assert_eq!(names.len(), 2);
    assert!(names.contains("bash"));
    assert!(names.contains("read"));
    assert!(!names.contains("write"));
    assert!(!names.contains("glob"));
    assert_eq!(executors.len(), 2);
    assert!(executors.contains_key("bash"));
    assert!(executors.contains_key("read"));
}

#[test]
fn tool_names_filter_single_tool() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "write"]);
    let mcp_map = HashMap::new();
    let filter = vec!["bash".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        None,
        Some(&filter),
    );

    assert!(result.is_ok());
    assert_eq!(names.len(), 1);
    assert!(names.contains("bash"));
    assert_eq!(executors.len(), 1);
}

#[test]
fn tool_names_filter_empty_array_means_no_tools() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "write"]);
    let mcp_map = HashMap::new();
    let filter: Vec<String> = vec![];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        None,
        Some(&filter),
    );

    assert!(result.is_ok());
    assert_eq!(names.len(), 0);
    assert_eq!(executors.len(), 0);
}

#[test]
fn tool_names_filter_unknown_tool_returns_error() {
    let (mut names, mut executors) = make_tools(&["bash", "read"]);
    let mcp_map = HashMap::new();
    let filter = vec!["bash".to_string(), "nonexistent".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        None,
        Some(&filter),
    );

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("nonexistent"),
        "error should name the tool: {err}"
    );
    assert!(err.contains("agent1"), "error should name the agent: {err}");
}

// ── filter_conversation_tools: mcp_server_names filter ────────────────────

#[test]
fn mcp_filter_keeps_only_specified_server_tools() {
    // Agent has builtin tools + tools from two MCP servers
    let (mut names, mut executors) =
        make_tools(&["bash", "read", "search", "query", "index", "delete"]);
    let mut mcp_map = HashMap::new();
    mcp_map.insert(
        "server-a".to_string(),
        vec!["search".to_string(), "query".to_string()],
    );
    mcp_map.insert(
        "server-b".to_string(),
        vec!["index".to_string(), "delete".to_string()],
    );
    let filter = vec!["server-a".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        Some(&filter),
        None,
    );

    assert!(result.is_ok());
    // Builtin tools (bash, read) remain, server-a tools (search, query) remain
    // server-b tools (index, delete) are removed
    assert_eq!(names.len(), 4);
    assert!(names.contains("bash"));
    assert!(names.contains("read"));
    assert!(names.contains("search"));
    assert!(names.contains("query"));
    assert!(!names.contains("index"));
    assert!(!names.contains("delete"));
    assert_eq!(executors.len(), 4);
}

#[test]
fn mcp_filter_empty_array_removes_all_mcp_tools() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "search", "index"]);
    let mut mcp_map = HashMap::new();
    mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
    mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);
    let filter: Vec<String> = vec![];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        Some(&filter),
        None,
    );

    assert!(result.is_ok());
    // Only builtin tools remain
    assert_eq!(names.len(), 2);
    assert!(names.contains("bash"));
    assert!(names.contains("read"));
    assert!(!names.contains("search"));
    assert!(!names.contains("index"));
}

#[test]
fn mcp_filter_unknown_server_returns_error() {
    let (mut names, mut executors) = make_tools(&["bash", "search"]);
    let mut mcp_map = HashMap::new();
    mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
    let filter = vec!["server-a".to_string(), "nonexistent-server".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        Some(&filter),
        None,
    );

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("nonexistent-server"),
        "error should name the server: {err}"
    );
    assert!(err.contains("agent1"), "error should name the agent: {err}");
}

// ── filter_conversation_tools: both filters combined ──────────────────────

#[test]
fn both_filters_mcp_applied_first_then_tool_names() {
    let (mut names, mut executors) = make_tools(&["bash", "read", "search", "query", "index"]);
    let mut mcp_map = HashMap::new();
    mcp_map.insert(
        "server-a".to_string(),
        vec!["search".to_string(), "query".to_string()],
    );
    mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);

    // MCP filter: only server-a → removes "index"
    // Tool filter: only "bash" and "search" → removes "read" and "query"
    let mcp_filter = vec!["server-a".to_string()];
    let tool_filter = vec!["bash".to_string(), "search".to_string()];

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        Some(&mcp_filter),
        Some(&tool_filter),
    );

    assert!(result.is_ok());
    assert_eq!(names.len(), 2);
    assert!(names.contains("bash"));
    assert!(names.contains("search"));
    assert_eq!(executors.len(), 2);
}

#[test]
fn tool_filter_referencing_mcp_tool_removed_by_server_filter_errors() {
    // MCP filter removes "index", then tool filter requests "index" → error
    let (mut names, mut executors) = make_tools(&["bash", "search", "index"]);
    let mut mcp_map = HashMap::new();
    mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
    mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);

    let mcp_filter = vec!["server-a".to_string()]; // removes "index"
    let tool_filter = vec!["bash".to_string(), "index".to_string()]; // requests "index"

    let result = filter_conversation_tools(
        "agent1",
        &mut names,
        &mut executors,
        &mcp_map,
        Some(&mcp_filter),
        Some(&tool_filter),
    );

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("index"),
        "error should name the unavailable tool: {err}"
    );
}
