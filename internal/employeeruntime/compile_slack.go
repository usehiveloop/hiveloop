package employeeruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func buildSlackConfig(ctx context.Context, deps CompileDeps, agent *model.Agent) SlackConfig {
	cfg := SlackConfig{PostableChannels: []SlackChannelSpec{}}
	if deps.DB == nil || deps.Nango == nil || agent == nil || agent.OrgID == nil {
		return cfg
	}
	botToken, err := loadSlackBotToken(ctx, deps, *agent.OrgID)
	if err != nil {
		return cfg
	}
	channelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	channels, err := slackapp.ListBotChannels(channelCtx, botToken)
	if err != nil {
		return cfg
	}
	cfg.PostableChannels = slackChannelSpecs(channels)
	return cfg
}

func slackChannelSpecs(channels []slackapp.Channel) []SlackChannelSpec {
	out := make([]SlackChannelSpec, 0, len(channels))
	for _, ch := range channels {
		description := strings.TrimSpace(ch.Topic)
		if description == "" {
			description = strings.TrimSpace(ch.Purpose)
		} else if purpose := strings.TrimSpace(ch.Purpose); purpose != "" && purpose != description {
			description += " " + purpose
		}
		out = append(out, SlackChannelSpec{
			ID:          ch.ID,
			Name:        ch.Name,
			Description: description,
			IsPrivate:   ch.IsPrivate,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func slackConfigMap(cfg SlackConfig) map[string]any {
	out := map[string]any{}
	if len(cfg.PostableChannels) == 0 {
		return out
	}
	channels := make([]map[string]any, 0, len(cfg.PostableChannels))
	for _, ch := range cfg.PostableChannels {
		item := map[string]any{
			"id":   ch.ID,
			"name": ch.Name,
		}
		if ch.Description != "" {
			item["description"] = ch.Description
		}
		if ch.IsPrivate {
			item["is_private"] = true
		}
		channels = append(channels, item)
	}
	out["postable_channels"] = channels
	return out
}

func loadSlackBotToken(ctx context.Context, deps CompileDeps, orgID uuid.UUID) (string, error) {
	if deps.DB == nil || deps.Nango == nil {
		return "", fmt.Errorf("employee runtime startup: db and nango client are required")
	}
	var conn model.InConnection
	if err := deps.DB.WithContext(ctx).
		Preload("InIntegration").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL AND in_integrations.provider = ?", orgID, slackapp.Provider).
		Order("in_connections.created_at ASC").
		First(&conn).Error; err != nil {
		return "", fmt.Errorf("active Slack connection required for employee sandbox startup: %w", err)
	}
	nangoConn, err := deps.Nango.GetConnection(ctx, conn.NangoConnectionID, "in_"+conn.InIntegration.UniqueKey)
	if err != nil {
		return "", fmt.Errorf("load Slack connection credentials: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, _ := creds[key].(string); strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("Slack connection credentials do not include a bot token")
}

func scopesFromIntegrations(integrations model.JSON) model.JSON {
	if len(integrations) == 0 {
		return nil
	}
	var scopes []map[string]any
	for connID, raw := range integrations {
		cfg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		actionsRaw, ok := cfg["actions"].([]any)
		if !ok {
			continue
		}
		var actions []string
		for _, a := range actionsRaw {
			if s, ok := a.(string); ok {
				actions = append(actions, s)
			}
		}
		if len(actions) > 0 {
			scopes = append(scopes, map[string]any{"connection_id": connID, "actions": actions})
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	return model.JSON{"scopes": scopes}
}
