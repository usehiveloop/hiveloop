use bridge_core::agent::AgentDefinition;
use bridge_core::BridgeError;

use crate::providers::{create_agent, BridgeAgent};
use crate::tool_adapter::DynamicTool;

/// Build a rig-core agent from an AgentDefinition and a set of tools.
///
/// Creates the provider client, configures the agent with the system prompt,
/// temperature, max_tokens, and tools, then returns the built Agent.
pub fn build_agent(
    definition: &AgentDefinition,
    tools: Vec<DynamicTool>,
) -> Result<BridgeAgent, BridgeError> {
    // Append a tool-use instruction to nudge models into producing a final text
    // response after tool calls (mitigates the empty-response-after-tools issue
    // seen across many providers).
    let preamble = format!(
        "{}\n\n[After using tools, always provide a final text response summarizing the results. Never end your turn with only tool calls and no text output.]",
        definition.system_prompt
    );

    create_agent(&definition.provider, tools, &preamble, definition)
}
