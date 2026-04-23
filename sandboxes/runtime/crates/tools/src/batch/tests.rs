use super::*;

/// A simple test tool that echoes its input.
struct EchoTool;

#[async_trait]
impl ToolExecutor for EchoTool {
    fn name(&self) -> &str {
        "echo"
    }
    fn description(&self) -> &str {
        "Echo tool for testing"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::json!({})
    }
    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        Ok(serde_json::to_string(&args).unwrap())
    }
    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

/// A tool that always fails.
struct FailTool;

#[async_trait]
impl ToolExecutor for FailTool {
    fn name(&self) -> &str {
        "fail"
    }
    fn description(&self) -> &str {
        "Fail tool for testing"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::json!({})
    }
    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        Err("intentional failure".to_string())
    }
    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

fn make_batch_tool() -> BatchTool {
    let mut tools: HashMap<String, Arc<dyn ToolExecutor>> = HashMap::new();
    tools.insert("echo".to_string(), Arc::new(EchoTool));
    tools.insert("fail".to_string(), Arc::new(FailTool));
    BatchTool::new(tools)
}

#[tokio::test]
async fn test_batch_basic() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "echo", "parameters": { "msg": "hello" } },
            { "tool": "echo", "parameters": { "msg": "world" } }
        ]
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: BatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total, 2);
    assert_eq!(parsed.succeeded, 2);
    assert_eq!(parsed.failed, 0);
}

#[tokio::test]
async fn test_batch_partial_failure() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "echo", "parameters": {} },
            { "tool": "fail", "parameters": {} }
        ]
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: BatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total, 2);
    assert_eq!(parsed.succeeded, 1);
    assert_eq!(parsed.failed, 1);
}

#[tokio::test]
async fn test_batch_tool_not_found() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "nonexistent", "parameters": {} }
        ]
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: BatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.failed, 1);
    let err_msg = parsed.results[0].error.as_ref().unwrap();
    assert!(
        err_msg.contains("not in registry"),
        "error should mention 'not in registry': {err_msg}"
    );
    assert!(
        err_msg.contains("Available tools:"),
        "error should list available tools: {err_msg}"
    );
}

#[tokio::test]
async fn test_batch_recursive_disallowed() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "batch", "parameters": {} }
        ]
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Recursive"));
}

#[tokio::test]
async fn test_batch_encouragement_on_success() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "echo", "parameters": { "msg": "a" } },
            { "tool": "echo", "parameters": { "msg": "b" } }
        ]
    });

    let result = tool.execute(args).await.expect("execute");
    assert!(
        result.contains("successfully"),
        "should have encouragement message: {result}"
    );
    assert!(
        result.contains("batch tool"),
        "should encourage batch usage: {result}"
    );
}

#[tokio::test]
async fn test_batch_no_encouragement_on_failure() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": [
            { "tool": "echo", "parameters": {} },
            { "tool": "fail", "parameters": {} }
        ]
    });

    let result = tool.execute(args).await.expect("execute");
    assert!(
        !result.contains("successfully"),
        "should NOT have encouragement on failure"
    );
}

#[tokio::test]
async fn test_batch_empty() {
    let tool = make_batch_tool();
    let args = serde_json::json!({
        "tool_calls": []
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("No tool calls"));
}
