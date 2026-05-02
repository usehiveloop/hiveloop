package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// applyAgentConfigDefaults populates the surviving Wave-2 AgentConfig fields
// (max_tokens, max_turns, temperature) with sensible per-provider defaults
// when the agent author didn't specify them. The dropped fields
// (max_tasks_per_conversation, history_strip,
// tool_requirements, immortal) are no longer applied here — the new ACP
// harness owns those concerns or they were retired entirely.
func applyAgentConfigDefaults(cfg *bridgepkg.AgentConfig, providerID, modelName string) *bridgepkg.AgentConfig {
	if cfg == nil {
		cfg = &bridgepkg.AgentConfig{}
	}

	if cfg.MaxTokens == nil {
		val := defaultMaxTokens(providerID, modelName)
		cfg.MaxTokens = &val
	}
	if cfg.MaxTurns == nil {
		val := int32(250)
		cfg.MaxTurns = &val
	}
	if cfg.Temperature == nil {
		temp := defaultTemperature(providerID, modelName)
		cfg.Temperature = &temp
	}

	return cfg
}

// applyHarnessOptionalFields pass-throughs harness-aware optional fields the
// agent author may have set in their JSONB AgentConfig: reasoning_effort,
// small_fast_model, fallback_model, allowed_tools, env, permission_mode.
// These are nil when the author did not specify them and the bridge/harness
// will fall back to its own defaults — we deliberately do NOT invent values
// here.
//
// NOTE: disabled_tools is populated in buildAgentDefinition from the
// per-tool permissions map (deny -> disabled), and is intentionally not
// touched here.
func applyHarnessOptionalFields(cfg *bridgepkg.AgentConfig, agentCfg *bridgepkg.AgentConfig) {
	if cfg == nil || agentCfg == nil {
		return
	}
	if cfg.ReasoningEffort == nil && agentCfg.ReasoningEffort != nil {
		cfg.ReasoningEffort = agentCfg.ReasoningEffort
	}
	if cfg.SmallFastModel == nil && agentCfg.SmallFastModel != nil {
		cfg.SmallFastModel = agentCfg.SmallFastModel
	}
	if cfg.FallbackModel == nil && agentCfg.FallbackModel != nil {
		cfg.FallbackModel = agentCfg.FallbackModel
	}
	if cfg.AllowedTools == nil && agentCfg.AllowedTools != nil {
		cfg.AllowedTools = agentCfg.AllowedTools
	}
	if cfg.PermissionMode == nil && agentCfg.PermissionMode != nil {
		cfg.PermissionMode = agentCfg.PermissionMode
	}
	if cfg.Env == nil && agentCfg.Env != nil {
		cfg.Env = agentCfg.Env
	}
}
