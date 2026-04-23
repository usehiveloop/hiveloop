package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	BridgePort = 25434

	bridgeHealthTimeout    = 90 * time.Second
	bridgeHealthInterval   = 2 * time.Second
	bridgeURLRefreshBuffer = 5 * time.Minute
	bridgeURLTTL           = 55 * time.Minute

	healthCheckInterval = 30 * time.Second
)

func baseEnvVars(cfg *config.Config, bridgeAPIKey string, sandboxID uuid.UUID, webhookURL string) map[string]string {
	envVars := map[string]string{
		"BRIDGE_CONTROL_PLANE_API_KEY": bridgeAPIKey,
		"BRIDGE_LISTEN_ADDR":           fmt.Sprintf("0.0.0.0:%d", BridgePort),
		"BRIDGE_LOG_FORMAT":            "json",
		"BRIDGE_STORAGE_PATH":          "/home/daytona/.bridge/storage",
		"BRIDGE_WEB_URL":               fmt.Sprintf("https://%s/spider", cfg.BridgeHost),
		"HIVELOOP_SANDBOX_ID":          sandboxID.String(),
	}
	if webhookURL != "" {
		envVars["BRIDGE_WEBHOOK_URL"] = webhookURL
	}
	return envVars
}

func setOrgEnvVars(envVars map[string]string, orgID uuid.UUID) {
	envVars["HIVELOOP_ORG_ID"] = orgID.String()
}

func setAgentEnvVars(envVars map[string]string, agent *model.Agent, cfg *config.Config) {
	if agent == nil {
		return
	}
	envVars["HIVELOOP_AGENT_ID"] = agent.ID.String()
	envVars["HIVELOOP_GIT_CREDENTIALS_URL"] = fmt.Sprintf("https://%s/internal/git-credentials/%s", cfg.BridgeHost, agent.ID)
	envVars["HIVELOOP_RAILWAY_API_URL"] = fmt.Sprintf("https://%s/internal/railway-proxy/%s", cfg.BridgeHost, agent.ID)
	envVars["HIVELOOP_RAILWAY_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["HIVELOOP_VERCEL_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["GH_NO_KEYRING"] = "1"
}

func setDriveEndpoint(envVars map[string]string, sandboxID uuid.UUID, cfg *config.Config) {
	envVars["HIVELOOP_DRIVE_ENDPOINT"] = fmt.Sprintf("https://%s/internal/sandbox-drive/%s", cfg.BridgeHost, sandboxID)
}

type repoResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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
