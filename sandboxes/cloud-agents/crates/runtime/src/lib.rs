//! Bridge runtime — stub shell.
//!
//! The in-house conversation loop, LLM dispatch, tools, and tool-call
//! enforcement have all been removed. The harness migration replaces them
//! with an external coding-agent process (Claude Code via ACP, eventually
//! OpenCode).
//!
//! What remains here is the supervisor surface that the HTTP API and the
//! `bridge` binary still link against. Methods that used to drive the model
//! return [`bridge_core::BridgeError::HarnessUnavailable`] until the ACP
//! adapter is wired up. Methods that only manage state (storing definitions,
//! listing agents) keep working so push/sync flows behave correctly.

pub mod agent_map;
pub mod agent_state;
pub mod supervisor;

pub use agent_map::AgentMap;
pub use agent_state::{AgentState, ConversationHandle};
pub use supervisor::AgentSupervisor;
