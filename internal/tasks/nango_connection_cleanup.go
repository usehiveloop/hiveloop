package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

// AgentProfileNangoCleanupHandler removes external Nango connections that were
// attached to deleted agent profiles.
type AgentProfileNangoCleanupHandler struct {
	nango *nango.Client
}

func NewAgentProfileNangoCleanupHandler(nangoClient *nango.Client) *AgentProfileNangoCleanupHandler {
	return &AgentProfileNangoCleanupHandler{nango: nangoClient}
}

func (h *AgentProfileNangoCleanupHandler) Handle(ctx context.Context, t *asynq.Task) error {
	if h.nango == nil {
		return fmt.Errorf("nango cleanup: nango client is not configured: %w", asynq.SkipRetry)
	}
	var payload AgentProfileNangoCleanupPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent profile nango cleanup payload: %w", err)
	}
	if len(payload.Connections) == 0 {
		return nil
	}

	var errs []error
	for _, target := range payload.Connections {
		target.ConnectionID = strings.TrimSpace(target.ConnectionID)
		target.ProviderConfigKey = strings.TrimSpace(target.ProviderConfigKey)
		if target.ConnectionID == "" || target.ProviderConfigKey == "" {
			continue
		}
		if err := h.nango.DeleteConnection(ctx, target.ConnectionID, target.ProviderConfigKey); err != nil {
			wrapped := fmt.Errorf("delete nango connection %s profile_id=%s provider=%s: %w", target.ConnectionID, target.ProfileID, target.Provider, err)
			logging.Capture(ctx, wrapped)
			errs = append(errs, wrapped)
			continue
		}
		logging.FromContext(ctx).InfoContext(ctx, "agent profile nango connection deleted",
			"agent_id", payload.AgentID,
			"profile_id", target.ProfileID,
			"provider", target.Provider,
			"nango_connection_id", target.ConnectionID,
			"provider_config_key", target.ProviderConfigKey,
		)
	}
	return errors.Join(errs...)
}
