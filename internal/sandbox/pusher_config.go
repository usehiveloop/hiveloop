package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

const (
	defaultSubagentTimeoutForegroundSecs = 900
	defaultSubagentTimeoutBackgroundSecs = 1800
	historyStripPinRecent                = 5
	historyStripAgeThreshold             = 3
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
	setDefaultI64 := func(ptr **int64, val int64) {
		if *ptr == nil {
			*ptr = &val
		}
	}

	setDefault(&cfg.MaxTokens, defaultMaxTokens(providerID, modelName))
	setDefault(&cfg.MaxTurns, 250)
	setDefault(&cfg.MaxTasksPerConversation, 50)
	setDefault(&cfg.MaxConcurrentConversations, 100)
	setDefaultI64(&cfg.SubagentTimeoutForegroundSecs, defaultSubagentTimeoutForegroundSecs)
	setDefaultI64(&cfg.SubagentTimeoutBackgroundSecs, defaultSubagentTimeoutBackgroundSecs)

	if cfg.Temperature == nil {
		temp := defaultTemperature(providerID, modelName)
		cfg.Temperature = &temp
	}

	return cfg
}

func applyHistoryStripDefault(cfg *bridgepkg.AgentConfig) {
	if cfg == nil || cfg.HistoryStrip != nil {
		return
	}
	enabled := true
	pinErrors := true
	pinRecent := historyStripPinRecent
	ageThreshold := historyStripAgeThreshold
	cfg.HistoryStrip = &bridgepkg.HistoryStripConfig{
		Enabled:        &enabled,
		PinErrors:      &pinErrors,
		PinRecentCount: &pinRecent,
		AgeThreshold:   &ageThreshold,
	}
}
