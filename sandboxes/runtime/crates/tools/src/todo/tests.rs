use super::*;
use crate::ToolExecutor;

#[tokio::test]
async fn test_todowrite_returns_correct_incomplete_count() {
    let tool = TodoWriteTool::new();
    let args = serde_json::json!({
        "todos": [
            { "content": "Step 1", "status": "completed", "priority": "high" },
            { "content": "Step 2", "status": "in_progress", "priority": "high" },
            { "content": "Step 3", "status": "pending", "priority": "medium" },
        ]
    });
    let result = tool.execute(args).await.unwrap();
    let parsed: TodoWriteResult = serde_json::from_str(&result).unwrap();
    assert!(parsed.ok);
    assert_eq!(parsed.incomplete_count, 2);
}

#[tokio::test]
async fn test_todowrite_empty_list() {
    let tool = TodoWriteTool::new();
    let args = serde_json::json!({ "todos": [] });
    let result = tool.execute(args).await.unwrap();
    let parsed: TodoWriteResult = serde_json::from_str(&result).unwrap();
    assert!(parsed.ok);
    assert_eq!(parsed.incomplete_count, 0);
}

#[tokio::test]
async fn test_todowrite_invalid_args() {
    let tool = TodoWriteTool::new();
    // Missing required 'todos' field
    let args = serde_json::json!({ "items": [] });
    let result = tool.execute(args).await;
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("Invalid arguments"));
}

#[tokio::test]
async fn test_todoread_empty() {
    let tool = TodoReadTool::new();
    let result = tool.execute(serde_json::json!({})).await.unwrap();
    let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
    assert!(parsed.todos.is_empty());
    assert_eq!(parsed.total, 0);
    assert_eq!(parsed.incomplete_count, 0);
}

#[tokio::test]
async fn test_todoread_after_write() {
    let state = TodoState::new();
    let write_tool = TodoWriteTool::with_state(state.clone());
    let read_tool = TodoReadTool::with_state(state);

    // Write some todos
    let args = serde_json::json!({
        "todos": [
            { "content": "Task A", "status": "pending", "priority": "high" },
            { "content": "Task B", "status": "completed", "priority": "low" },
        ]
    });
    write_tool.execute(args).await.unwrap();

    // Read them back
    let result = read_tool.execute(serde_json::json!({})).await.unwrap();
    let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
    assert_eq!(parsed.total, 2);
    assert_eq!(parsed.incomplete_count, 1);
    assert_eq!(parsed.todos[0].content, "Task A");
    assert_eq!(parsed.todos[1].content, "Task B");
}

#[tokio::test]
async fn test_todoread_shared_state() {
    let state = TodoState::new();
    let write_tool = TodoWriteTool::with_state(state.clone());
    let read_tool = TodoReadTool::with_state(state);

    // Initially empty
    let result = read_tool.execute(serde_json::json!({})).await.unwrap();
    let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
    assert_eq!(parsed.total, 0);

    // Write
    write_tool
        .execute(serde_json::json!({
            "todos": [{ "content": "X", "status": "pending", "priority": "medium" }]
        }))
        .await
        .unwrap();

    // Read reflects write
    let result = read_tool.execute(serde_json::json!({})).await.unwrap();
    let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
    assert_eq!(parsed.total, 1);

    // Overwrite with empty
    write_tool
        .execute(serde_json::json!({ "todos": [] }))
        .await
        .unwrap();

    // Read reflects empty
    let result = read_tool.execute(serde_json::json!({})).await.unwrap();
    let parsed: TodoReadResult = serde_json::from_str(&result).unwrap();
    assert_eq!(parsed.total, 0);
}
