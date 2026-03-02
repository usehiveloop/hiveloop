use bridge_core::BridgeError;

use crate::diff::AgentDiff;

/// Apply a computed diff to the supervisor.
///
/// Delegates to [`runtime::AgentSupervisor::apply_diff`] which handles
/// adding new agents, draining and replacing updated agents, and
/// removing stale agents.
pub async fn apply_diff(
    supervisor: &runtime::AgentSupervisor,
    diff: AgentDiff,
) -> Result<(), BridgeError> {
    supervisor
        .apply_diff(diff.added, diff.updated, diff.removed)
        .await
}
