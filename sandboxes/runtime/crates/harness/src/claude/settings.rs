//! Write Claude Code's `settings.json` from the AgentConfig before
//! spawning the agent process.
//!
//! claude-agent-acp ignores the `permissionMode` field on the SDK Options
//! and reads `permissions.defaultMode` from `$CLAUDE_CONFIG_DIR/settings.json`
//! instead. We materialize the file so the active mode tracks the agent
//! definition.

use bridge_core::AgentDefinition;
use serde_json::{json, Map, Value};
use std::path::Path;
use tracing::warn;

pub fn write_settings(config_dir: &Path, agent: &AgentDefinition) {
    if let Err(e) = std::fs::create_dir_all(config_dir) {
        warn!(path = %config_dir.display(), error = %e, "settings dir creation failed");
        return;
    }

    let mut root = Map::new();

    // permissions block
    let mut permissions = Map::new();
    if let Some(mode) = &agent.config.permission_mode {
        permissions.insert("defaultMode".to_string(), json!(mode));
    }
    if !agent.config.allowed_tools.is_empty() {
        permissions.insert(
            "allow".to_string(),
            Value::Array(
                agent
                    .config
                    .allowed_tools
                    .iter()
                    .map(|t| Value::String(t.clone()))
                    .collect(),
            ),
        );
    }
    if !agent.config.disabled_tools.is_empty() {
        permissions.insert(
            "deny".to_string(),
            Value::Array(
                agent
                    .config
                    .disabled_tools
                    .iter()
                    .map(|t| Value::String(t.clone()))
                    .collect(),
            ),
        );
    }
    if !permissions.is_empty() {
        root.insert("permissions".to_string(), Value::Object(permissions));
    }

    if !agent.config.env.is_empty() {
        let env = agent
            .config
            .env
            .iter()
            .map(|(k, v)| (k.clone(), json!(v)))
            .collect::<Map<_, _>>();
        root.insert("env".to_string(), Value::Object(env));
    }

    let path = config_dir.join("settings.json");
    let body =
        serde_json::to_string_pretty(&Value::Object(root)).unwrap_or_else(|_| "{}".to_string());
    if let Err(e) = std::fs::write(&path, body) {
        warn!(path = %path.display(), error = %e, "settings.json write failed");
    }
}
