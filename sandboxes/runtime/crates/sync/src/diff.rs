use std::collections::HashMap;

use bridge_core::{AgentDefinition, AgentSummary};

/// Describes the changes between the currently loaded agents and the
/// latest set fetched from the control plane.
#[derive(Debug, Clone, PartialEq)]
pub struct AgentDiff {
    /// Agents that are new and need to be loaded.
    pub added: Vec<AgentDefinition>,
    /// Agents whose version has changed and need to be reloaded.
    pub updated: Vec<AgentDefinition>,
    /// Agent IDs that are no longer present and should be removed.
    pub removed: Vec<String>,
}

impl AgentDiff {
    /// Returns `true` if the diff contains no changes at all.
    pub fn is_empty(&self) -> bool {
        self.added.is_empty() && self.updated.is_empty() && self.removed.is_empty()
    }
}

/// Compare the currently loaded agents against the latest definitions
/// fetched from the control plane and produce a diff.
///
/// - A fetched agent whose ID does not appear in `current` is **added**.
/// - A current agent whose ID does not appear in `fetched` is **removed**.
/// - A fetched agent whose ID exists in `current` but with a different
///   `version` is **updated**.
pub fn compute_diff(current: &[AgentSummary], fetched: &[AgentDefinition]) -> AgentDiff {
    let current_map: HashMap<&str, Option<&str>> = current
        .iter()
        .map(|a| (a.id.as_str(), a.version.as_deref()))
        .collect();

    let fetched_map: HashMap<&str, &AgentDefinition> =
        fetched.iter().map(|a| (a.id.as_str(), a)).collect();

    let mut added = Vec::new();
    let mut updated = Vec::new();

    for def in fetched {
        match current_map.get(def.id.as_str()) {
            None => {
                // New agent
                added.push(def.clone());
            }
            Some(current_version) => {
                // Same ID — check if version differs
                if *current_version != def.version.as_deref() {
                    updated.push(def.clone());
                }
            }
        }
    }

    let removed: Vec<String> = current
        .iter()
        .filter(|a| !fetched_map.contains_key(a.id.as_str()))
        .map(|a| a.id.clone())
        .collect();

    AgentDiff {
        added,
        updated,
        removed,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::agent::AgentConfig;
    use bridge_core::provider::{ProviderConfig, ProviderType};
    use pretty_assertions::assert_eq;

    fn make_summary(id: &str, version: &str) -> AgentSummary {
        AgentSummary {
            id: id.to_string(),
            name: format!("Agent {id}"),
            version: Some(version.to_string()),
        }
    }

    fn make_definition(id: &str, version: &str) -> AgentDefinition {
        AgentDefinition {
            id: id.to_string(),
            name: format!("Agent {id}"),
            system_prompt: "test".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "test-key".to_string(),
                base_url: None,
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            webhook_url: None,
            webhook_secret: None,
            version: Some(version.to_string()),
            updated_at: None,
        }
    }

    #[test]
    fn test_new_agent_detected() {
        let current: Vec<AgentSummary> = vec![];
        let fetched = vec![make_definition("agent1", "1")];

        let diff = compute_diff(&current, &fetched);

        assert_eq!(diff.added.len(), 1);
        assert_eq!(diff.added[0].id, "agent1");
        assert!(diff.updated.is_empty());
        assert!(diff.removed.is_empty());
    }

    #[test]
    fn test_removed_agent_detected() {
        let current = vec![make_summary("agent1", "1")];
        let fetched: Vec<AgentDefinition> = vec![];

        let diff = compute_diff(&current, &fetched);

        assert!(diff.added.is_empty());
        assert!(diff.updated.is_empty());
        assert_eq!(diff.removed, vec!["agent1".to_string()]);
    }

    #[test]
    fn test_updated_agent_different_version() {
        let current = vec![make_summary("agent1", "1")];
        let fetched = vec![make_definition("agent1", "2")];

        let diff = compute_diff(&current, &fetched);

        assert!(diff.added.is_empty());
        assert_eq!(diff.updated.len(), 1);
        assert_eq!(diff.updated[0].id, "agent1");
        assert_eq!(diff.updated[0].version, Some("2".to_string()));
        assert!(diff.removed.is_empty());
    }

    #[test]
    fn test_no_changes_returns_empty() {
        let current = vec![make_summary("agent1", "1"), make_summary("agent2", "3")];
        let fetched = vec![
            make_definition("agent1", "1"),
            make_definition("agent2", "3"),
        ];

        let diff = compute_diff(&current, &fetched);

        assert!(diff.is_empty());
    }

    #[test]
    fn test_mix_of_operations() {
        let current = vec![
            make_summary("keep", "1"),
            make_summary("update_me", "1"),
            make_summary("remove_me", "1"),
        ];
        let fetched = vec![
            make_definition("keep", "1"),
            make_definition("update_me", "2"),
            make_definition("new_agent", "1"),
        ];

        let diff = compute_diff(&current, &fetched);

        assert_eq!(diff.added.len(), 1);
        assert_eq!(diff.added[0].id, "new_agent");

        assert_eq!(diff.updated.len(), 1);
        assert_eq!(diff.updated[0].id, "update_me");

        assert_eq!(diff.removed.len(), 1);
        assert_eq!(diff.removed[0], "remove_me");
    }

    #[test]
    fn test_is_empty_true_when_all_vecs_empty() {
        let diff = AgentDiff {
            added: vec![],
            updated: vec![],
            removed: vec![],
        };
        assert!(diff.is_empty());
    }

    #[test]
    fn test_is_empty_false_with_added() {
        let diff = AgentDiff {
            added: vec![make_definition("a", "1")],
            updated: vec![],
            removed: vec![],
        };
        assert!(!diff.is_empty());
    }

    #[test]
    fn test_is_empty_false_with_updated() {
        let diff = AgentDiff {
            added: vec![],
            updated: vec![make_definition("a", "1")],
            removed: vec![],
        };
        assert!(!diff.is_empty());
    }

    #[test]
    fn test_is_empty_false_with_removed() {
        let diff = AgentDiff {
            added: vec![],
            updated: vec![],
            removed: vec!["a".to_string()],
        };
        assert!(!diff.is_empty());
    }
}
