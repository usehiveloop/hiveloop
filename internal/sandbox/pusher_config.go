package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

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

// applyHarnessOptionalFields propagates harness-aware optional fields from
// the author's AgentConfig. Leave nil values nil so the harness applies its
// own defaults. disabled_tools is set from the permissions map in
// buildAgentDefinition and is intentionally not touched here.
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
	if cfg.PermissionMode == nil && agentCfg.PermissionMode != nil {
		cfg.PermissionMode = agentCfg.PermissionMode
	}
	if cfg.Env == nil && agentCfg.Env != nil {
		cfg.Env = agentCfg.Env
	}
}
