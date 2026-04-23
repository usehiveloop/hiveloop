//! Shared helpers for `ToolCallEmitter` unit tests.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use dashmap::DashMap;
use tokio_util::sync::CancellationToken;
use tools::ToolExecutor;
use webhooks::EventBus;

use crate::permission_manager::PermissionManager;
use crate::tool_hook::ToolCallEmitter;

pub(super) type TestModel =
    <rig::providers::openai::CompletionsClient as rig::prelude::CompletionClient>::CompletionModel;

pub(super) fn make_bus() -> Arc<EventBus> {
    Arc::new(EventBus::new(None, None, String::new(), String::new()))
}

pub(super) fn make_emitter(bus: Arc<EventBus>) -> ToolCallEmitter {
    make_emitter_with(
        bus,
        HashSet::new(),
        HashMap::<String, Arc<dyn ToolExecutor>>::new(),
    )
}

pub(super) fn make_emitter_with(
    bus: Arc<EventBus>,
    tool_names: HashSet<String>,
    tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
) -> ToolCallEmitter {
    ToolCallEmitter {
        event_bus: bus,
        cancel: CancellationToken::new(),
        tool_names,
        tool_executors,
        agent_id: "test-agent".to_string(),
        conversation_id: "test-conv".to_string(),
        permission_manager: Arc::new(PermissionManager::new()),
        agent_permissions: HashMap::new(),
        metrics: Arc::new(bridge_core::AgentMetrics::new()),
        conversation_metrics: None,
        pending_tool_timings: Arc::new(DashMap::new()),
        storage: None,
        persisted_messages: None,
        pressure_threshold_bytes: None,
        pressure_counter: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        pressure_warned: Arc::new(std::sync::atomic::AtomicBool::new(false)),
    }
}
