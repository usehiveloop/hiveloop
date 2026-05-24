# Adding a Tool

Step-by-step guide to adding a new tool to Bridge.

---

## Overview

To add a tool:

1. Define the tool struct
2. Implement the `Tool` trait
3. Register the tool
4. Add tests
5. Update documentation

---

## Step 1: Create the Tool File

Create `crates/tools/src/my_tool.rs`:

```rust
use serde::{Deserialize, Serialize};
use crate::{Tool, ToolContext, ToolResult};

#[derive(Debug, Deserialize, JsonSchema)]
pub struct MyToolArgs {
    /// Description of parameter
    pub param: String,
}

pub struct MyTool;

#[async_trait]
impl Tool for MyTool {
    fn name(&self) -> &str {
        "my_tool"
    }

    fn description(&self) -> &str {
        "What this tool does"
    }

    fn schema(&self) -> serde_json::Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "param": {
                    "type": "string",
                    "description": "Description of parameter"
                }
            },
            "required": ["param"]
        })
    }

    async fn execute(&self, ctx: &ToolContext, args: MyToolArgs) -> ToolResult {
        // Tool logic here
        let result = do_something(args.param).await?;
        
        Ok(ToolOutput {
            success: true,
            result: result.into(),
            error: None,
        })
    }
}

// Register the tool
inventory::submit! {
    crate::ToolDefinition::new("my_tool", || Box::new(MyTool))
}
```

---

## Step 2: Add to Module

In `crates/tools/src/lib.rs`:

```rust
pub mod my_tool;
```

---

## Step 3: Implement the Logic

Fill in `execute()`:

```rust
async fn execute(&self, ctx: &ToolContext, args: MyToolArgs) -> ToolResult {
    // Validate input
    if args.param.is_empty() {
        return Err(ToolError::InvalidInput("param cannot be empty"));
    }
    
    // Do work
    let result = perform_action(&args.param).await
        .map_err(|e| ToolError::Execution(e.to_string()))?;
    
    // Return result
    Ok(ToolOutput {
        success: true,
        result: serde_json::json!(result),
        error: None,
    })
}
```

---

## Step 4: Add Tests

In `crates/tools/src/my_tool.rs`:

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_my_tool_success() {
        let tool = MyTool;
        let ctx = ToolContext::default();
        let args = MyToolArgs {
            param: "test".to_string(),
        };
        
        let result = tool.execute(&ctx, args).await.unwrap();
        
        assert!(result.success);
    }

    #[tokio::test]
    async fn test_my_tool_empty_param() {
        let tool = MyTool;
        let ctx = ToolContext::default();
        let args = MyToolArgs {
            param: "".to_string(),
        };
        
        let result = tool.execute(&ctx, args).await;
        
        assert!(result.is_err());
    }
}
```

---

## Step 5: Run Tests

```bash
cargo test -p tools my_tool
```

---

## Step 6: Document

Add to documentation:

1. Update `docs/tools-reference/index.md`
2. Create `docs/tools-reference/my-tool.md`
3. Add example to tutorials if useful

---

## Tool Context

The `ToolContext` provides:

| Field | Description |
|-------|-------------|
| `agent_id` | Current agent ID |
| `conversation_id` | Current conversation ID |
| `working_dir` | Working directory path |

---

## Error Handling

Use `ToolError` variants:

| Error | Use When |
|-------|----------|
| `InvalidInput` | Bad arguments |
| `Execution` | Runtime error |
| `PermissionDenied` | Not allowed |
| `NotFound` | Resource missing |

---

## Complete Example

See existing tools for patterns:

- `read.rs` — Simple file reading
- `bash.rs` — Command execution
- `spawn_agent.rs` — Complex with subagents

---

## See Also

- [Tools Reference](../tools-reference/index.md)
- [Tools Concept](../core-concepts/tools.md)
