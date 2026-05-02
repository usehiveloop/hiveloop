use bridge_core::AgentDefinition;

pub(super) fn definitions_equivalent(
    existing: &AgentDefinition,
    incoming: &AgentDefinition,
) -> bool {
    match (&existing.version, &incoming.version) {
        (Some(existing_version), Some(incoming_version)) => existing_version == incoming_version,
        _ => existing == incoming,
    }
}
