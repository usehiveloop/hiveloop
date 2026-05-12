package slack

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	Kind               = "SLACK_BOT_PROFILE"
	defaultHistoryDays = 90
	minHistoryDays     = 60
	maxHistoryDays     = 90
)

type Config struct {
	AgentProfileID               string `json:"agent_profile_id"`
	AgentID                      string `json:"agent_id"`
	HistoryDays                  int    `json:"history_days"`
	IncludePublicChannels        bool   `json:"include_public_channels"`
	IncludeJoinedPrivateChannels bool   `json:"include_joined_private_channels"`
}

func LoadConfig(raw json.RawMessage) (Config, error) {
	cfg := Config{
		HistoryDays:                  defaultHistoryDays,
		IncludePublicChannels:        true,
		IncludeJoinedPrivateChannels: true,
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("slack: parse config: %w", err)
		}
	}
	if _, err := uuid.Parse(cfg.AgentProfileID); err != nil {
		return Config{}, fmt.Errorf("slack: agent_profile_id is required")
	}
	if cfg.AgentID != "" {
		if _, err := uuid.Parse(cfg.AgentID); err != nil {
			return Config{}, fmt.Errorf("slack: agent_id must be a valid UUID")
		}
	}
	if cfg.HistoryDays == 0 {
		cfg.HistoryDays = defaultHistoryDays
	}
	if cfg.HistoryDays < minHistoryDays || cfg.HistoryDays > maxHistoryDays {
		return Config{}, fmt.Errorf("slack: history_days must be between %d and %d", minHistoryDays, maxHistoryDays)
	}
	return cfg, nil
}

func (c Config) profileUUID() uuid.UUID {
	id, _ := uuid.Parse(c.AgentProfileID)
	return id
}
