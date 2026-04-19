pub mod cache_resource_pool;
pub mod factory;
pub mod permission_manager;
pub mod prefix_hash;
pub mod providers;
pub mod streaming;
pub mod tool_adapter;
pub mod tool_hook;

pub use factory::build_agent;
pub use permission_manager::PermissionManager;
pub use prefix_hash::{
    compute_preamble_hash, compute_prefix_hash, compute_tools_hash, prefix_hash_from_definitions,
    split_hashes_from_definitions, suspected_volatile_markers, ToolPrefix,
};
pub use providers::{BridgeAgent, BridgeStream, BridgeStreamItem, PromptResponse};
pub use tool_adapter::{adapt_tools, DynamicTool};
pub use tool_hook::ToolCallEmitter;
