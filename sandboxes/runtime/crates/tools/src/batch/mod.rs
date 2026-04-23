use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;

use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// A single tool call within a batch.
#[derive(Debug, Clone, Deserialize, JsonSchema)]
pub struct BatchToolCall {
    /// The name of the tool to call.
    pub tool: String,
    /// The parameters to pass to the tool.
    pub parameters: serde_json::Value,
}

/// Arguments for the Batch tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct BatchArgs {
    /// The list of tool calls to execute concurrently.
    pub tool_calls: Vec<BatchToolCall>,
}

/// Result of a single tool call within a batch.
#[derive(Debug, Serialize, Deserialize)]
pub struct BatchCallResult {
    pub success: bool,
    pub tool: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Result returned by the Batch tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct BatchResult {
    pub results: Vec<BatchCallResult>,
    pub total: usize,
    pub succeeded: usize,
    pub failed: usize,
}

/// Maximum number of tool calls per batch.
const MAX_BATCH_SIZE: usize = 25;

pub struct BatchTool {
    tools: HashMap<String, Arc<dyn ToolExecutor>>,
}

impl BatchTool {
    /// Create a new BatchTool with a snapshot of available tools.
    pub fn new(tools: HashMap<String, Arc<dyn ToolExecutor>>) -> Self {
        Self { tools }
    }
}

#[async_trait]
impl ToolExecutor for BatchTool {
    fn name(&self) -> &str {
        "batch"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/batch.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(BatchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: BatchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.tool_calls.is_empty() {
            return Err("No tool calls provided".to_string());
        }

        // Separate calls within limit and discarded calls beyond limit
        let all_calls = args.tool_calls;
        let (calls, discarded): (Vec<_>, Vec<_>) = {
            let mut within = Vec::new();
            let mut beyond = Vec::new();
            for (i, call) in all_calls.into_iter().enumerate() {
                if i < MAX_BATCH_SIZE {
                    within.push(call);
                } else {
                    beyond.push(call);
                }
            }
            (within, beyond)
        };

        // Disallow recursive batch calls
        for call in &calls {
            if call.tool == "batch" {
                return Err("Recursive batch calls are not allowed".to_string());
            }
        }

        // Execute all calls concurrently
        let futures: Vec<_> = calls
            .into_iter()
            .map(|call| {
                let tool_name = call.tool.clone();
                let params = call.parameters.clone();
                let tools = &self.tools;

                async move {
                    let tool = match tools.get(&tool_name) {
                        Some(t) => t,
                        None => {
                            const FILTERED_FROM_SUGGESTIONS: &[&str] =
                                &["invalid", "patch", "batch", "apply_patch"];
                            let available: Vec<&str> = tools
                                .keys()
                                .map(|k| k.as_str())
                                .filter(|k| !FILTERED_FROM_SUGGESTIONS.contains(k))
                                .collect();
                            let msg = format!(
                                "Tool '{}' not in registry. External tools (MCP, environment) cannot be batched \u{2014} call them directly. Available tools: {}",
                                tool_name,
                                available.join(", ")
                            );
                            return BatchCallResult {
                                success: false,
                                tool: tool_name,
                                result: None,
                                error: Some(msg),
                            };
                        }
                    };

                    match tool.execute(params).await {
                        Ok(output) => {
                            // Try to parse the output as JSON for structured results
                            let value = serde_json::from_str::<serde_json::Value>(&output)
                                .unwrap_or(serde_json::Value::String(output));
                            BatchCallResult {
                                success: true,
                                tool: tool_name,
                                result: Some(value),
                                error: None,
                            }
                        }
                        Err(e) => BatchCallResult {
                            success: false,
                            tool: tool_name,
                            result: None,
                            error: Some(e),
                        },
                    }
                }
            })
            .collect();

        let mut results = futures::future::join_all(futures).await;

        // Add error results for discarded calls beyond MAX_BATCH_SIZE
        for discarded_call in &discarded {
            results.push(BatchCallResult {
                success: false,
                tool: discarded_call.tool.clone(),
                result: None,
                error: Some(format!(
                    "Maximum of {} tools allowed in batch",
                    MAX_BATCH_SIZE
                )),
            });
        }

        let total = results.len();
        let succeeded = results.iter().filter(|r| r.success).count();
        let failed = total - succeeded;

        let batch_result = BatchResult {
            results,
            total,
            succeeded,
            failed,
        };

        let serialized = if failed == 0 {
            let msg = format!(
                "All {} tools executed successfully.\n\nKeep using the batch tool for optimal performance!",
                succeeded
            );
            serde_json::json!({
                "results": batch_result.results,
                "total": batch_result.total,
                "succeeded": batch_result.succeeded,
                "failed": batch_result.failed,
                "message": msg,
            })
            .to_string()
        } else {
            serde_json::to_string(&batch_result)
                .map_err(|e| format!("Failed to serialize result: {e}"))?
        };

        // Apply shared truncation for large aggregated results
        let truncated = crate::truncation::truncate_output(
            &serialized,
            crate::truncation::MAX_LINES,
            crate::truncation::MAX_BYTES,
        );
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
