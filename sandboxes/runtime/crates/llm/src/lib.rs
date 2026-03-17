pub mod factory;
pub mod permission_manager;
pub mod providers;
pub mod streaming;
pub mod tool_adapter;
pub mod tool_hook;

pub use factory::build_agent;
pub use permission_manager::PermissionManager;
pub use providers::{BridgeAgent, PromptResponse};
pub use streaming::{SseEvent, TokenUsage};
pub use tool_adapter::{adapt_tools, DynamicTool};
pub use tool_hook::ToolCallEmitter;
