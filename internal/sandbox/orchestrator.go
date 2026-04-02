package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/bridge"
	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/turso"
)

const (
	// BridgePort is the fixed port Bridge listens on inside every sandbox.
	BridgePort = 25434

	// bridgeHealthTimeout is the max time to wait for Bridge to become healthy.
	bridgeHealthTimeout = 90 * time.Second

	// bridgeHealthInterval is the polling interval for Bridge health checks.
	bridgeHealthInterval = 2 * time.Second

	// bridgeURLRefreshBuffer is how early we refresh the pre-auth URL before it expires.
	bridgeURLRefreshBuffer = 5 * time.Minute

	// bridgeURLTTL is how long we assume a pre-auth URL is valid.
	// Daytona signed URLs last ~60 minutes; we store 55 to refresh early.
	bridgeURLTTL = 55 * time.Minute

	// healthCheckInterval is how often the background health checker runs.
	healthCheckInterval = 30 * time.Second
)

// Orchestrator manages sandbox lifecycle — creating, starting, stopping sandboxes
// and providing BridgeClients to talk to them.
type Orchestrator struct {
	db       *gorm.DB
	provider Provider
	turso    *turso.Provisioner
	encKey   *crypto.SymmetricKey
	cfg      *config.Config
}

// NewOrchestrator creates a sandbox orchestrator.
func NewOrchestrator(db *gorm.DB, provider Provider, turso *turso.Provisioner, encKey *crypto.SymmetricKey, cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		db:       db,
		provider: provider,
		turso:    turso,
		encKey:   encKey,
		cfg:      cfg,
	}
}

// EnsureSharedSandbox returns the identity's shared sandbox, creating or waking it if needed.
// This is synchronous — it blocks until the sandbox is running or returns an error.
func (o *Orchestrator) EnsureSharedSandbox(ctx context.Context, org *model.Org, identity *model.Identity) (*model.Sandbox, error) {
	// Check for existing shared sandbox for this identity
	var existing model.Sandbox
	err := o.db.Where("identity_id = ? AND sandbox_type = 'shared'", identity.ID).First(&existing).Error
	if err == nil {
		// Verify the sandbox still exists in the provider
		if err := o.verifySandboxExists(ctx, &existing); err != nil {
			slog.Warn("shared sandbox stale, deleting and recreating",
				"sandbox_id", existing.ID,
				"external_id", existing.ExternalID,
				"error", err,
			)
			o.db.Delete(&existing)
			return o.createSandbox(ctx, org, identity, "shared", nil)
		}

		switch existing.Status {
		case "running":
			return &existing, nil
		case "stopped":
			return o.wakeSandbox(ctx, &existing)
		default:
			// creating, starting, error — try to wake anyway
			return o.wakeSandbox(ctx, &existing)
		}
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("querying shared sandbox: %w", err)
	}

	// No existing sandbox — create a new one
	return o.createSandbox(ctx, org, identity, "shared", nil)
}

// verifySandboxExists checks if the sandbox's external ID still exists in the provider.
func (o *Orchestrator) verifySandboxExists(ctx context.Context, sb *model.Sandbox) error {
	if sb.ExternalID == "" {
		return fmt.Errorf("no external ID")
	}
	_, err := o.provider.GetEndpoint(ctx, sb.ExternalID, BridgePort)
	return err
}

// CreateDedicatedSandbox spins up a new sandbox for a dedicated agent.
// Synchronous — blocks until running or returns an error.
func (o *Orchestrator) CreateDedicatedSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	// Load org and identity
	var org model.Org
	if err := o.db.Where("id = ?", agent.OrgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("loading org: %w", err)
	}
	var identity model.Identity
	if err := o.db.Where("id = ?", agent.IdentityID).First(&identity).Error; err != nil {
		return nil, fmt.Errorf("loading identity: %w", err)
	}

	return o.createSandbox(ctx, &org, &identity, "dedicated", agent)
}

// GetBridgeClient returns a BridgeClient connected to the sandbox.
// If the pre-auth URL is expired or about to expire, it refreshes it first.
func (o *Orchestrator) GetBridgeClient(ctx context.Context, sb *model.Sandbox) (*bridge.BridgeClient, error) {
	// Decrypt the Bridge API key
	apiKey, err := o.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting bridge api key: %w", err)
	}

	// Check if URL needs refresh
	if o.needsURLRefresh(sb) {
		if err := o.refreshBridgeURL(ctx, sb); err != nil {
			return nil, fmt.Errorf("refreshing bridge URL: %w", err)
		}
	}

	return bridge.NewBridgeClient(sb.BridgeURL, apiKey), nil
}

// StopSandbox stops a running sandbox.
func (o *Orchestrator) StopSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("stopping sandbox %s: %w", sb.ID, err)
	}
	return o.db.Model(sb).Updates(map[string]any{
		"status":    "stopped",
		"bridge_url_expires_at": nil,
	}).Error
}

// DeleteSandbox tears down a sandbox via the provider and removes the DB record.
func (o *Orchestrator) DeleteSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil {
		slog.Warn("failed to delete sandbox from provider", "sandbox_id", sb.ID, "external_id", sb.ExternalID, "error", err)
		// Continue to delete DB record even if provider fails
	}
	return o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error
}

// StartHealthChecker runs a background goroutine that periodically syncs sandbox
// status from the provider and auto-stops idle sandboxes.
func (o *Orchestrator) StartHealthChecker(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("sandbox health checker stopped")
			return
		case <-ticker.C:
			o.runHealthCheck(ctx)
		}
	}
}

// --- internal helpers ---

func (o *Orchestrator) createSandbox(ctx context.Context, org *model.Org, identity *model.Identity, sandboxType string, agent *model.Agent) (*model.Sandbox, error) {
	// Ensure Turso storage for the org (optional — Bridge works without it)
	var storageURL, authToken string
	if o.turso != nil {
		var err error
		storageURL, authToken, err = o.turso.EnsureStorage(ctx, org.ID)
		if err != nil {
			slog.Warn("turso storage provisioning failed, continuing without libsql", "error", err)
		}
	}

	// Generate and encrypt Bridge API key
	bridgeAPIKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating bridge api key: %w", err)
	}
	encryptedKey, err := o.encKey.EncryptString(bridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting bridge api key: %w", err)
	}

	// Build sandbox ID (we create the record first to get the UUID for webhook URL)
	sb := model.Sandbox{
		OrgID:                 org.ID,
		IdentityID:            identity.ID,
		SandboxType:           sandboxType,
		ExternalID:            "", // set after provider creates it
		BridgeURL:             "", // set after we get the endpoint
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if agent != nil {
		sb.AgentID = &agent.ID
		if agent.SandboxTemplateID != nil {
			sb.SandboxTemplateID = agent.SandboxTemplateID
		}
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox record: %w", err)
	}

	// Build env vars for Bridge
	envVars := map[string]string{
		"BRIDGE_CONTROL_PLANE_API_KEY": bridgeAPIKey,
		"BRIDGE_LISTEN_ADDR":          fmt.Sprintf("0.0.0.0:%d", BridgePort),
		"BRIDGE_WEBHOOK_URL":          fmt.Sprintf("https://%s/internal/webhooks/bridge/%s", o.cfg.BridgeHost, sb.ID),
		"BRIDGE_LOG_FORMAT":           "json",
	}
	if storageURL != "" {
		envVars["BRIDGE_STORAGE_URL"] = storageURL
		envVars["BRIDGE_STORAGE_AUTH_TOKEN"] = authToken
	}

	// Resolve snapshot
	snapshotID := o.resolveSnapshot(agent)

	// Build sandbox name
	name := o.buildSandboxName(sandboxType, identity, agent)

	// Build labels
	labels := map[string]string{
		"org_id":       org.ID.String(),
		"identity_id":  identity.ID.String(),
		"sandbox_type": sandboxType,
		"sandbox_id":   sb.ID.String(),
	}
	if agent != nil {
		labels["agent_id"] = agent.ID.String()
	}

	// Create via provider (synchronous — blocks until running)
	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       name,
		SnapshotID: snapshotID,
		EnvVars:    envVars,
		Labels:     labels,
	})
	if err != nil {
		// Clean up DB record on failure
		o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		return nil, fmt.Errorf("creating sandbox via provider: %w", err)
	}

	// Get pre-authenticated endpoint URL
	bridgeURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, BridgePort)
	if err != nil {
		// Sandbox was created but we can't reach it — update with error
		o.db.Model(&sb).Updates(map[string]any{
			"external_id":    info.ExternalID,
			"status":         "error",
			"error_message":  fmt.Sprintf("failed to get endpoint: %v", err),
		})
		return nil, fmt.Errorf("getting sandbox endpoint: %w", err)
	}

	slog.Info("got bridge endpoint",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"bridge_url", bridgeURL,
	)

	now := time.Now()
	expiresAt := now.Add(bridgeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":           info.ExternalID,
		"bridge_url":            bridgeURL,
		"bridge_url_expires_at": expiresAt,
		"status":                "running",
		"last_active_at":        now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating sandbox record: %w", err)
	}

	sb.ExternalID = info.ExternalID
	sb.BridgeURL = bridgeURL
	sb.BridgeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now

	// Wait for Bridge to become healthy (it starts automatically via entrypoint)
	if err := o.waitForBridgeHealthy(ctx, &sb); err != nil {
		o.db.Model(&sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("bridge failed to start: %v", err),
		})
		return nil, fmt.Errorf("waiting for bridge: %w", err)
	}

	slog.Info("sandbox created",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"type", sandboxType,
		"identity_id", identity.ID,
	)

	return &sb, nil
}

func (o *Orchestrator) wakeSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return nil, fmt.Errorf("starting sandbox %s: %w", sb.ID, err)
	}

	if err := o.refreshBridgeURL(ctx, sb); err != nil {
		return nil, fmt.Errorf("refreshing bridge URL after wake: %w", err)
	}

	now := time.Now()
	o.db.Model(sb).Updates(map[string]any{
		"status":        "running",
		"last_active_at": now,
		"error_message": nil,
	})
	sb.Status = "running"
	sb.LastActiveAt = &now

	// Wait for Bridge to become healthy (it restarts automatically via entrypoint)
	if err := o.waitForBridgeHealthy(ctx, sb); err != nil {
		o.db.Model(sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("bridge not healthy after wake: %v", err),
		})
		return nil, fmt.Errorf("bridge not healthy after wake: %w", err)
	}

	slog.Info("sandbox woken", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return sb, nil
}

func (o *Orchestrator) needsURLRefresh(sb *model.Sandbox) bool {
	if sb.BridgeURL == "" {
		return true
	}
	if sb.BridgeURLExpiresAt == nil {
		return true
	}
	return time.Now().Add(bridgeURLRefreshBuffer).After(*sb.BridgeURLExpiresAt)
}

func (o *Orchestrator) refreshBridgeURL(ctx context.Context, sb *model.Sandbox) error {
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, BridgePort)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(bridgeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"bridge_url":            url,
		"bridge_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("updating bridge URL: %w", err)
	}
	sb.BridgeURL = url
	sb.BridgeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) resolveSnapshot(agent *model.Agent) string {
	if agent != nil && agent.SandboxTemplateID != nil {
		var tmpl model.SandboxTemplate
		if err := o.db.Where("id = ?", *agent.SandboxTemplateID).First(&tmpl).Error; err == nil {
			if tmpl.ExternalID != nil && tmpl.BuildStatus == "ready" {
				return *tmpl.ExternalID
			}
		}
	}
	return o.cfg.BridgeBaseImagePrefix
}

func (o *Orchestrator) buildSandboxName(sandboxType string, identity *model.Identity, agent *model.Agent) string {
	short := shortID(identity.ID)
	if sandboxType == "dedicated" && agent != nil {
		// Sanitize agent name for use in sandbox name
		safeName := sanitizeName(agent.Name)
		return fmt.Sprintf("llmv-ded-%s-%s", safeName, shortID(agent.ID))
	}
	return fmt.Sprintf("llmv-sh-%s-%s", sanitizeName(identity.ExternalID), short)
}

func (o *Orchestrator) runHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.Where("status = 'running'").Find(&sandboxes).Error; err != nil {
		slog.Error("health check: failed to query sandboxes", "error", err)
		return
	}

	for i := range sandboxes {
		sb := &sandboxes[i]
		o.checkSandboxHealth(ctx, sb)
	}
}

func (o *Orchestrator) checkSandboxHealth(ctx context.Context, sb *model.Sandbox) {
	// Sync status from provider
	status, err := o.provider.GetStatus(ctx, sb.ExternalID)
	if err != nil {
		slog.Warn("health check: failed to get status", "sandbox_id", sb.ID, "error", err)
		return
	}

	providerStatus := string(status)
	if providerStatus != sb.Status {
		slog.Info("health check: status changed", "sandbox_id", sb.ID, "old", sb.Status, "new", providerStatus)
		o.db.Model(sb).Update("status", providerStatus)
		sb.Status = providerStatus
	}

	// Auto-stop idle sandboxes
	if sb.Status != "running" || sb.LastActiveAt == nil {
		return
	}

	idleMinutes := time.Since(*sb.LastActiveAt).Minutes()
	var threshold int
	if sb.SandboxType == "shared" {
		threshold = o.cfg.SharedSandboxIdleTimeoutMins
	} else {
		threshold = o.cfg.DedicatedSandboxGracePeriodMins
	}

	if threshold > 0 && int(idleMinutes) >= threshold {
		slog.Info("health check: auto-stopping idle sandbox",
			"sandbox_id", sb.ID, "type", sb.SandboxType, "idle_mins", int(idleMinutes))
		if err := o.StopSandbox(ctx, sb); err != nil {
			slog.Error("health check: failed to stop sandbox", "sandbox_id", sb.ID, "error", err)
		}
	}
}

// waitForBridgeHealthy polls Bridge's /health endpoint until it responds 200 or timeout.
func (o *Orchestrator) waitForBridgeHealthy(ctx context.Context, sb *model.Sandbox) error {
	healthURL := sb.BridgeURL + "/health"
	deadline := time.Now().Add(bridgeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	slog.Info("waiting for bridge healthy",
		"sandbox_id", sb.ID,
		"health_url", healthURL,
		"bridge_url", sb.BridgeURL,
	)

	for time.Now().Before(deadline) {
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("creating health request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				slog.Info("bridge healthy",
					"sandbox_id", sb.ID,
					"attempts", attempt,
					"elapsed", time.Since(deadline.Add(-bridgeHealthTimeout)).String(),
				)
				return nil
			}
			slog.Info("bridge health check: non-200", "status", resp.StatusCode, "attempt", attempt, "url", healthURL)
		} else {
			slog.Info("bridge health check: connection failed", "attempt", attempt, "url", healthURL, "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bridgeHealthInterval):
		}
	}

	return fmt.Errorf("bridge did not become healthy within %s (%d attempts)", bridgeHealthTimeout, attempt)
}

// ExecuteCommand runs a command inside a sandbox via the provider.
func (o *Orchestrator) ExecuteCommand(ctx context.Context, sb *model.Sandbox, command string) (string, error) {
	return o.provider.ExecuteCommand(ctx, sb.ExternalID, command)
}

// BuildTemplate builds a sandbox template (snapshot) via the provider.
// Runs asynchronously — updates the template record with build status.
func (o *Orchestrator) BuildTemplate(ctx context.Context, tmpl *model.SandboxTemplate) {
	o.db.Model(tmpl).Update("build_status", "building")

	snapshotName := fmt.Sprintf("llmv-tmpl-%s", shortID(tmpl.ID))
	externalID, err := o.provider.BuildSnapshot(ctx, BuildSnapshotOpts{
		Name:          snapshotName,
		BuildCommands: tmpl.BuildCommands,
	})

	if err != nil {
		errMsg := err.Error()
		o.db.Model(tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		slog.Error("template build failed", "template_id", tmpl.ID, "error", err)
		return
	}

	o.db.Model(tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	slog.Info("template built", "template_id", tmpl.ID, "external_id", externalID)
}

// DeleteTemplate deletes a sandbox template (snapshot) from the provider.
func (o *Orchestrator) DeleteTemplate(ctx context.Context, externalID string) error {
	return o.provider.DeleteSnapshot(ctx, externalID)
}

// --- utilities ---

func generateRandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func shortID(id uuid.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")[:12]
}

func sanitizeName(name string) string {
	// Keep only alphanumeric + hyphens, lowercase, truncate
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 20 {
		s = s[:20]
	}
	return s
}
