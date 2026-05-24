//! Bridge harness layer.
//!
//! Adapts external coding-agent CLIs to Bridge's supervisor surface using
//! the Agent Client Protocol (ACP) over stdio. The shared driver lives in
//! [`acp_session`]; per-harness modules ([`claude`], [`opencode`]) handle
//! spawn + per-harness config materialization. The supervisor calls
//! [`spawn`] which dispatches based on `agent.harness`.

pub mod acp_session;
pub mod claude;
pub mod events;
pub mod opencode;
pub mod skills;
pub(crate) mod stderr;

pub use acp_session::{AcpSession, ConversationContext, HarnessAdapter};

use bridge_core::agent::Harness;
use bridge_core::{AgentDefinition, BridgeError};
use std::sync::Arc;
use webhooks::{EventBus, PermissionManager};

/// Spawn the appropriate harness for the agent and return the
/// long-lived [`AcpSession`] handle.
pub async fn spawn(
    agent: AgentDefinition,
    event_bus: Arc<EventBus>,
    permission_manager: Arc<PermissionManager>,
) -> Result<Arc<AcpSession>, BridgeError> {
    match agent.harness {
        Harness::Claude => {
            let opts = claude::ClaudeHarnessOptions::from_env();
            claude::spawn(agent, opts, event_bus, permission_manager).await
        }
        Harness::OpenCode => {
            let opts = opencode::OpenCodeHarnessOptions::from_env();
            opencode::spawn(agent, opts, event_bus, permission_manager).await
        }
    }
}
