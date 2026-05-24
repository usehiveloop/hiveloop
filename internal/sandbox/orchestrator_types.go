package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

const (
	// BridgePort overrides the bridge binary's 8080 default via
	// BRIDGE_LISTEN_ADDR. Kept at the original hivy port (25434).
	BridgePort = 25434

	bridgeHealthTimeout    = 90 * time.Second
	bridgeHealthInterval   = 2 * time.Second
	bridgeURLRefreshBuffer = 5 * time.Minute
	bridgeURLTTL           = 55 * time.Minute

	healthCheckInterval = 30 * time.Second
)

func baseEnvVars(cfg *config.Config, bridgeAPIKey string, sandboxID uuid.UUID, webhookURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	envVars := map[string]string{
		"BRIDGE_CONTROL_PLANE_API_KEY":          bridgeAPIKey,
		employeeruntime.EmployeeEnvUploadBearer: bridgeAPIKey,
		"BRIDGE_LISTEN_ADDR":                    fmt.Sprintf("0.0.0.0:%d", BridgePort),
		"BRIDGE_LOG_FORMAT":                     "json",
		"BRIDGE_WEB_URL":                        controlPlaneBaseURL + "/spider",
		employeeruntime.EmployeeEnvSandboxID:    sandboxID.String(),
		// HOME=/work so bridge.db survives provider stop/start.
		employeeruntime.EmployeeEnvHome: "/work",
		"OPENCODE_CONFIG_DIR":           "/work/.opencode",
		"NO_BROWSER":                    "1",
		// SQLite persistence for bridge conversation/session state. /work is
		// HOME and survives provider stop/start.
		"BRIDGE_STORAGE_PATH": "/work/bridge.db",
	}
	if webhookURL != "" {
		envVars["BRIDGE_WEBHOOK_URL"] = webhookURL
	}
	agentSentryDSN := ""
	if cfg != nil {
		agentSentryDSN = cfg.AgentSandboxSentryDSN
	}
	setSandboxSentryEnvVars(envVars, cfg, agentSentryDSN)
	return envVars
}

func setSandboxSentryEnvVars(envVars map[string]string, cfg *config.Config, dsn string) {
	if cfg == nil || strings.TrimSpace(dsn) == "" {
		return
	}
	envVars[employeeruntime.EmployeeEnvSentryDSN] = strings.TrimSpace(dsn)
	envVars[employeeruntime.EmployeeEnvSentryEnvironment] = cfg.Environment
	envVars[employeeruntime.EmployeeEnvSentrySampleRate] = "1"
	envVars[employeeruntime.EmployeeEnvSentryTracesSampleRate] = strconv.FormatFloat(cfg.SentryTracesSampleRate, 'f', -1, 64)
	envVars[employeeruntime.EmployeeEnvSentryEnableLogs] = "true"
	if strings.TrimSpace(cfg.SentryRelease) != "" {
		envVars[employeeruntime.EmployeeEnvSentryRelease] = cfg.SentryRelease
	}
}

func setOrgEnvVars(envVars map[string]string, orgID uuid.UUID) {
	envVars[employeeruntime.EmployeeEnvOrgID] = orgID.String()
}

func setAgentEnvVars(envVars map[string]string, agent *model.Employee, cfg *config.Config) {
	if agent == nil {
		return
	}
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	envVars[employeeruntime.EmployeeEnvHivyEmployeeID] = agent.ID.String()
	envVars[employeeruntime.EmployeeEnvGitCredentialsURL] = fmt.Sprintf("%s/internal/git-credentials/%s", controlPlaneBaseURL, agent.ID)
	envVars[employeeruntime.EmployeeEnvBugsinkURL] = fmt.Sprintf("%s/internal/bugsink-proxy/%s", controlPlaneBaseURL, agent.ID)
	envVars[employeeruntime.EmployeeEnvBugsinkToken] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars[employeeruntime.EmployeeEnvLinearURL] = fmt.Sprintf("%s/internal/linear-proxy/%s", controlPlaneBaseURL, agent.ID)
	envVars[employeeruntime.EmployeeEnvLinearToken] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars[employeeruntime.EmployeeEnvNotionAPIURL] = fmt.Sprintf("%s/internal/notion-proxy/%s", controlPlaneBaseURL, agent.ID)
	envVars[employeeruntime.EmployeeEnvNotionToken] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["HIVY_RAILWAY_API_URL"] = fmt.Sprintf("%s/internal/railway-proxy/%s", controlPlaneBaseURL, agent.ID)
	envVars["HIVY_RAILWAY_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["HIVY_VERCEL_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars[employeeruntime.EmployeeEnvGitHubNoKeyring] = "1"
}

func setDriveEndpoint(envVars map[string]string, sandboxID uuid.UUID, cfg *config.Config) {
	envVars["HIVY_DRIVE_ENDPOINT"] = fmt.Sprintf("%s/internal/sandbox-drive/%s", cfg.RuntimeControlPlaneBaseURL(), sandboxID)
}

// setAssetsUploadURL exposes the conversation-asset endpoint base. The
// bridge appends the per-session conversation_id and the agent's chosen
// "<folder>/<filename>" tail so the final PUT URL is:
//
//	$HIVY_ASSETS_UPLOAD_URL/{conversationID}/assets/{folder}/{filename}
//
// Auth uses the same bridge API key already exported as
// BRIDGE_CONTROL_PLANE_API_KEY.
func setAssetsUploadURL(envVars map[string]string, cfg *config.Config) {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	envVars["HIVY_ASSETS_UPLOAD_URL"] = controlPlaneBaseURL + "/internal/conversations"
	envVars["HIVY_EMPLOYEE_ASSETS_UPLOAD_URL"] = controlPlaneBaseURL + "/internal/employees"
}

func employeeDriveUploadURL(cfg *config.Config, employeeID uuid.UUID, folder string) string {
	base := fmt.Sprintf("%s/internal/employees/%s/assets", cfg.RuntimeControlPlaneBaseURL(), employeeID)
	folder = strings.Trim(strings.TrimSpace(folder), "/")
	if folder == "" {
		return base
	}
	return base + "/" + folder
}

func setEmployeeDriveUploadURL(envVars map[string]string, cfg *config.Config, employeeID uuid.UUID, folder string) {
	envVars[employeeruntime.EmployeeEnvDriveUploadURL] = employeeDriveUploadURL(cfg, employeeID, folder)
}

func setUploadBearer(envVars map[string]string, bearer string) {
	envVars[employeeruntime.EmployeeEnvUploadBearer] = bearer
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
