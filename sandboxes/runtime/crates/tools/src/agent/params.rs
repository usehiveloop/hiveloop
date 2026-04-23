use schemars::JsonSchema;
use serde::Deserialize;

/// Parameters for the sub_agent tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct SubAgentToolParams {
    /// Short (3-5 word) description of the task.
    #[schemars(description = "Short (3-5 word) description of the task. Example: 'Fix login bug'")]
    pub description: String,
    /// The detailed task for the subagent to perform.
    #[schemars(
        description = "The detailed task for the subagent to perform. Be specific and include all necessary context"
    )]
    pub prompt: String,
    /// Which subagent to invoke (must match a defined subagent name).
    #[schemars(description = "Which subagent to invoke. Must match an available subagent name")]
    pub subagent_name: String,
    /// Set to true to run in background (returns immediately; result is injected
    /// into the next user turn as a `[Background Agent Task Completed]` message).
    #[schemars(
        description = "Set to true to run in background. Returns immediately with task_id; the final result is automatically injected into the next user turn when the subagent finishes."
    )]
    #[serde(default)]
    pub run_in_background: bool,
    /// Resume a previous subagent session by task_id.
    #[schemars(description = "Resume a previous subagent session by providing its task_id")]
    #[serde(default)]
    pub task_id: Option<String>,
}
