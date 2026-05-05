package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	HermesSidecarPort    = 7777
	hermesHealthTimeout  = 90 * time.Second
	hermesHealthInterval = 2 * time.Second
)

func (o *Orchestrator) CreateHermesSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("CreateHermesSandbox: agent must have org_id")
	}
	orgID := *agent.OrgID

	apiKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating sidecar api key: %w", err)
	}
	encryptedKey, err := o.encKey.EncryptString(apiKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting sidecar api key: %w", err)
	}

	sb := model.Sandbox{
		OrgID:                 &orgID,
		AgentID:               &agent.ID,
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox: %w", err)
	}

	envVars := hermesEnvVars(o.cfg, apiKey, &sb, orgID, agent)
	labels := map[string]string{
		"org_id":     orgID.String(),
		"sandbox_id": sb.ID.String(),
		"agent_id":   agent.ID.String(),
		"harness":    "hermes",
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       buildHermesSandboxName(agent),
		SnapshotID: o.cfg.HermesBaseImagePrefix,
		EnvVars:    envVars,
		Labels:     labels,
	})
	if err != nil {
		if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned sandbox row after provider create failure",
				"error", delErr, "sandbox_id", sb.ID)
		}
		return nil, fmt.Errorf("provider create: %w", err)
	}

	sandboxURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, HermesSidecarPort)
	if err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"external_id":   info.ExternalID,
			"status":        "error",
			"error_message": "get endpoint failed",
		})
		return nil, fmt.Errorf("getting sidecar endpoint: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(bridgeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":           info.ExternalID,
		"bridge_url":            sandboxURL,
		"bridge_url_expires_at": expiresAt,
		"status":                "running",
		"last_active_at":        now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating sandbox: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.BridgeURL = sandboxURL
	sb.BridgeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now

	if err := o.waitForSidecarReady(ctx, &sb, apiKey); err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"status":        "error",
			"error_message": "sidecar not ready",
		})
		return nil, fmt.Errorf("waiting for sidecar: %w", err)
	}

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)

	logging.FromContext(ctx).InfoContext(ctx, "hermes sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	return &sb, nil
}

func hermesEnvVars(cfg *config.Config, apiKey string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Agent) map[string]string {
	nameSlug := sanitizeName(agent.Name)
	if nameSlug == "" {
		nameSlug = "agent"
	}
	return map[string]string{
		"AGENT_ID":                     agent.ID.String(),
		"CONTROL_PLANE_URL":            "https://" + cfg.BridgeHost,
		"CONTROL_PLANE_API_KEY":        apiKey,
		"HIVELOOP_GIT_USERNAME":        nameSlug,
		"HIVELOOP_GIT_EMAIL":           nameSlug + "@usehiveloop.com",
		"HIVELOOP_GIT_CREDENTIALS_URL": fmt.Sprintf("https://%s/internal/git-credentials/%s", cfg.BridgeHost, agent.ID),
		"HIVELOOP_SANDBOX_ID":          sb.ID.String(),
		"HIVELOOP_ORG_ID":              orgID.String(),
	}
}

func (o *Orchestrator) markSandboxError(ctx context.Context, sb *model.Sandbox, updates map[string]any) {
	if err := o.db.Model(sb).Updates(updates).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "mark sandbox error",
			"error", err, "sandbox_id", sb.ID)
	}
}

func buildHermesSandboxName(agent *model.Agent) string {
	return fmt.Sprintf("hiveloop-hermes-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}

// EnsureHermesSandboxURL refreshes the sandbox's pre-authenticated URL when
// it's expired (or about to). Mirrors refreshBridgeURL but routes to the
// sidecar port (7777) instead of the bridge port (25434). Without this, the
// stale URL falls through Daytona's auth gate and returns the dex login HTML.
func (o *Orchestrator) EnsureHermesSandboxURL(ctx context.Context, sb *model.Sandbox) error {
	if !o.needsURLRefresh(sb) {
		return nil
	}
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, HermesSidecarPort)
	if err != nil {
		return fmt.Errorf("get hermes sandbox endpoint: %w", err)
	}
	expiresAt := time.Now().Add(bridgeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"bridge_url":            url,
		"bridge_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("update sandbox url: %w", err)
	}
	sb.BridgeURL = url
	sb.BridgeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) waitForSidecarReady(ctx context.Context, sb *model.Sandbox, apiKey string) error {
	statusURL := strings.TrimRight(sb.BridgeURL, "/") + "/v1/hermes/status"
	deadline := time.Now().Add(hermesHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	logging.FromContext(ctx).InfoContext(ctx, "waiting for hermes sidecar", "sandbox_id", sb.ID)

	for time.Now().Before(deadline) {
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, doErr := client.Do(req)
		if doErr != nil {
			logging.FromContext(ctx).DebugContext(ctx, "hermes sidecar probe transport error",
				"sandbox_id", sb.ID, "attempt", attempt, "error", doErr)
		} else {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "hermes sidecar ready",
					"sandbox_id", sb.ID, "attempts", attempt)
				return nil
			}
			logging.FromContext(ctx).DebugContext(ctx, "hermes sidecar probe non-200",
				"sandbox_id", sb.ID, "attempt", attempt, "status", status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(hermesHealthInterval):
		}
	}
	return fmt.Errorf("sidecar not ready within %s (%d attempts)", hermesHealthTimeout, attempt)
}
