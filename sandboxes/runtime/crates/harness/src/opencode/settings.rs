//! Materialize an [`AgentDefinition`] into opencode's per-agent
//! `opencode.json` and a sibling `instructions.md` (the latter holds the
//! system prompt because opencode's `instructions` config field expects
//! file paths, not inline text).

use bridge_core::mcp::{McpServerDefinition, McpTransport};
use bridge_core::AgentDefinition;
use serde_json::{json, Map, Value};
use std::path::{Path, PathBuf};
use tracing::warn;

/// Write `opencode.json` + `instructions.md` under `config_dir` and
/// return the absolute path to the JSON file (for `OPENCODE_CONFIG`).
///
/// The instructions file is written into `cwd/.bridge-instructions.md`
/// because opencode resolves `instructions` paths relative to `cwd`.
pub fn write_config(
    config_dir: &Path,
    cwd: &Path,
    agent: &AgentDefinition,
) -> Result<PathBuf, String> {
    std::fs::create_dir_all(config_dir).map_err(|e| {
        warn!(path = %config_dir.display(), error = %e, "opencode config dir mkdir failed");
        format!("opencode config dir mkdir failed: {e}")
    })?;
    std::fs::create_dir_all(cwd).map_err(|e| {
        warn!(path = %cwd.display(), error = %e, "opencode cwd mkdir failed");
        format!("opencode cwd mkdir failed: {e}")
    })?;

    // Write the system prompt to a sibling file so it loads via opencode's
    // `instructions` array. Empty prompt → skip.
    let mut instructions = Vec::<String>::new();
    if !agent.system_prompt.trim().is_empty() {
        let prompt_path = cwd.join(".bridge-instructions.md");
        if let Err(e) = std::fs::write(&prompt_path, &agent.system_prompt) {
            warn!(path = %prompt_path.display(), error = %e, "instructions file write failed");
        } else {
            instructions.push(prompt_path.to_string_lossy().to_string());
        }
    }

    let mut root = Map::new();
    root.insert(
        "$schema".to_string(),
        json!("https://opencode.ai/config.json"),
    );

    // Model id. ProviderConfig stores `model` as the bare model id; opencode
    // expects `provider/model` (e.g. `anthropic/claude-sonnet-4-5`). If the
    // caller already encodes the slash, leave it; otherwise prepend the
    // ProviderType tag.
    let provider_id = provider_to_opencode(&agent.provider.provider_type);
    let model_id = if agent.provider.model.contains('/') {
        agent.provider.model.clone()
    } else {
        format!("{provider_id}/{}", agent.provider.model)
    };
    root.insert("model".to_string(), json!(model_id));

    if let Some(small) = &agent.config.small_fast_model {
        let small_id = if small.contains('/') {
            small.clone()
        } else {
            format!("{provider_id}/{small}")
        };
        root.insert("small_model".to_string(), json!(small_id));
    }

    // Pass an explicit provider entry when the caller supplied a base URL
    // override or runs through opencode's built-in providers but needs an
    // api key threaded in. For ProviderType::Custom we wire the openai-
    // compatible AI SDK and register the model in the provider's `models`
    // map so opencode's strict provider/model validation accepts it.
    if agent.provider.base_url.is_some()
        || matches!(
            agent.provider.provider_type,
            bridge_core::provider::ProviderType::Custom
        )
    {
        let mut provider_block = Map::new();
        let mut entry = Map::new();
        let is_custom = matches!(
            agent.provider.provider_type,
            bridge_core::provider::ProviderType::Custom
        );
        if is_custom {
            entry.insert("npm".to_string(), json!("@ai-sdk/openai-compatible"));
            entry.insert("name".to_string(), json!(provider_id));
            // opencode rejects a `model` reference whose model id isn't
            // listed under the provider's `models` map. Register the bare
            // model id with empty overrides so opencode treats it as
            // available. Callers can extend this later.
            let mut models = Map::new();
            let bare = strip_provider_prefix(&model_id);
            models.insert(bare.to_string(), json!({}));
            entry.insert("models".to_string(), Value::Object(models));
        }
        let mut options = Map::new();
        if let Some(base) = &agent.provider.base_url {
            options.insert("baseURL".to_string(), json!(base));
        }
        if !agent.provider.api_key.is_empty() {
            options.insert("apiKey".to_string(), json!(agent.provider.api_key));
        }
        if !options.is_empty() {
            entry.insert("options".to_string(), Value::Object(options));
        }
        provider_block.insert(provider_id.to_string(), Value::Object(entry));
        root.insert("provider".to_string(), Value::Object(provider_block));
    }

    if !instructions.is_empty() {
        root.insert("instructions".to_string(), json!(instructions));
    }

    if !agent.mcp_servers.is_empty() {
        root.insert(
            "mcp".to_string(),
            Value::Object(translate_mcp(&agent.mcp_servers)),
        );
    }

    if let Some(perm) = build_permission_block(agent) {
        root.insert("permission".to_string(), perm);
    }

    if !agent.config.disabled_tools.is_empty() {
        let mut tools = Map::new();
        for t in &agent.config.disabled_tools {
            tools.insert(t.clone(), Value::Bool(false));
        }
        root.insert("tools".to_string(), Value::Object(tools));
    }

    // Skills materialized to disk under `<config_dir>/skills/<id>/SKILL.md`
    // by `skills::write_skills`. opencode's auto-discovery scans
    // `config.directories()` for `{skill,skills}/**/SKILL.md`, so writing
    // to OPENCODE_CONFIG_DIR is usually sufficient — but we also add the
    // dir to `skills.paths` so the discovery walk hits it deterministically
    // even when opencode's `config.directories()` resolution misses.
    if !agent.skills.is_empty() {
        let skills_dir = config_dir.join("skills");
        let skills_dir_s = skills_dir.to_string_lossy().to_string();
        let mut skills_block = Map::new();
        skills_block.insert("paths".to_string(), json!([skills_dir_s]));
        root.insert("skills".to_string(), Value::Object(skills_block));
    }

    let path = config_dir.join("opencode.json");
    let body = serde_json::to_string_pretty(&Value::Object(root))
        .map_err(|e| format!("opencode.json serialize failed: {e}"))?;
    std::fs::write(&path, body).map_err(|e| {
        warn!(path = %path.display(), error = %e, "opencode.json write failed");
        format!("opencode.json write failed: {e}")
    })?;
    Ok(path)
}

fn translate_mcp(servers: &[McpServerDefinition]) -> Map<String, Value> {
    let mut out = Map::new();
    for s in servers {
        let entry = match &s.transport {
            McpTransport::Stdio { command, args, env } => {
                // opencode's mcp.<name>.command is a single string array
                // (not separate command + args). Build it joined.
                let mut cmd = vec![command.clone()];
                cmd.extend(args.iter().cloned());
                let mut env_obj = Map::new();
                for (k, v) in env {
                    env_obj.insert(k.clone(), json!(v));
                }
                json!({
                    "type": "local",
                    "command": cmd,
                    "environment": Value::Object(env_obj),
                    "enabled": true,
                })
            }
            McpTransport::StreamableHttp { url, headers } => {
                let mut header_obj = Map::new();
                for (k, v) in headers {
                    header_obj.insert(k.clone(), json!(v));
                }
                json!({
                    "type": "remote",
                    "url": url,
                    "headers": Value::Object(header_obj),
                    "enabled": true,
                })
            }
        };
        out.insert(s.name.clone(), entry);
    }
    out
}

fn build_permission_block(agent: &AgentDefinition) -> Option<Value> {
    // opencode's permission schema differs from Claude Code's. We map the
    // closest equivalents; agents that need fine-grained control can pass
    // through any opencode-native permission schema via env-injected config
    // files later.
    let mut perm = Map::new();
    if let Some(mode) = &agent.config.permission_mode {
        // opencode supports per-action defaults (`edit`, `bash`, `webfetch`).
        // Rough translation: bypassPermissions → "allow" everywhere;
        // acceptEdits → "allow" for edit; default/plan → "ask".
        let default_action = match mode.as_str() {
            "bypassPermissions" | "auto" => "allow",
            "acceptEdits" => "allow",
            _ => "ask",
        };
        perm.insert("edit".to_string(), json!(default_action));
        perm.insert("bash".to_string(), json!(default_action));
        perm.insert("webfetch".to_string(), json!(default_action));
    }
    if perm.is_empty() {
        None
    } else {
        Some(Value::Object(perm))
    }
}

fn strip_provider_prefix(model_id: &str) -> &str {
    model_id.split_once('/').map(|(_, m)| m).unwrap_or(model_id)
}

fn provider_to_opencode(p: &bridge_core::provider::ProviderType) -> &'static str {
    use bridge_core::provider::ProviderType;
    match p {
        ProviderType::Anthropic => "anthropic",
        ProviderType::OpenAI => "openai",
        ProviderType::Google => "google",
        ProviderType::Groq => "groq",
        ProviderType::DeepSeek => "deepseek",
        ProviderType::Mistral => "mistral",
        ProviderType::Cohere => "cohere",
        ProviderType::XAi => "xai",
        ProviderType::Together => "togetherai",
        ProviderType::Fireworks => "fireworks",
        ProviderType::Ollama => "ollama",
        ProviderType::Custom => "custom",
    }
}
