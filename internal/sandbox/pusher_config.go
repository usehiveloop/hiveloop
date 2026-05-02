package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// TODO(wave-2): The old bridge's AgentConfig carried a number of fields that
// the new ACP-harness OpenAPI dropped:
//   - max_tasks_per_conversation, max_concurrent_conversations
//   - subagent_timeout_foreground_secs, subagent_timeout_background_secs
//   - history_strip (PinErrors, PinRecentCount, AgeThreshold, Enabled)
//
// All of those defaults used to be applied here. Wave 2 will reintroduce
// equivalent behavior at the harness-adapter layer (subagent timeouts) or
// as a bridge-side preprocessor (history-strip / concurrency caps). For
// Wave 1 we only emit the fields that survived the new schema:
// MaxTokens, MaxTurns, Temperature.
const (
	historyStripPinRecent    = 5
	historyStripAgeThreshold = 3
)

func applyAgentConfigDefaults(cfg *bridgepkg.AgentConfig, providerID, modelName string) *bridgepkg.AgentConfig {
	if cfg == nil {
		cfg = &bridgepkg.AgentConfig{}
	}

	setDefault := func(ptr **int32, val int32) {
		if *ptr == nil {
			*ptr = &val
		}
	}

	setDefault(&cfg.MaxTokens, defaultMaxTokens(providerID, modelName))
	setDefault(&cfg.MaxTurns, 250)

	if cfg.Temperature == nil {
		temp := defaultTemperature(providerID, modelName)
		cfg.Temperature = &temp
	}

	return cfg
}

// applyHistoryStripDefault is a no-op in Wave 1 — see file-level TODO.
func applyHistoryStripDefault(_ *bridgepkg.AgentConfig) {
}
