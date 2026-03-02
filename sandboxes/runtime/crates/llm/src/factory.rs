use bridge_core::agent::AgentDefinition;
use bridge_core::BridgeError;

use crate::providers::{create_agent_builder, BridgeAgent};
use crate::tool_adapter::DynamicTool;

/// Build a rig-core agent from an AgentDefinition and a set of tools.
///
/// Creates the provider client, configures the agent with the system prompt,
/// temperature, max_tokens, and tools, then returns the built Agent.
pub fn build_agent(
    definition: &AgentDefinition,
    tools: Vec<DynamicTool>,
) -> Result<BridgeAgent, BridgeError> {
    let builder = create_agent_builder(&definition.provider)?;

    // Set system prompt and configuration
    let builder = builder.preamble(&definition.system_prompt);

    let builder = if let Some(temp) = definition.config.temperature {
        builder.temperature(temp)
    } else {
        builder
    };

    let builder = if let Some(max_tokens) = definition.config.max_tokens {
        builder.max_tokens(max_tokens as u64)
    } else {
        builder
    };

    let builder = if let Some(max_turns) = definition.config.max_turns {
        builder.default_max_turns(max_turns as usize)
    } else {
        builder
    };

    // Build with or without tools
    if tools.is_empty() {
        Ok(builder.build())
    } else {
        // Add the first tool to transition to WithBuilderTools state
        let mut iter = tools.into_iter();
        let first = iter.next().expect("checked non-empty above");
        let mut builder = builder.tool(first);

        // Add remaining tools
        for tool in iter {
            builder = builder.tool(tool);
        }

        Ok(builder.build())
    }
}
