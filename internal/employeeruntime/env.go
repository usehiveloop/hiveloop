package employeeruntime

import (
	"sort"
	"strings"
)

const (
	EmployeeEnvRuntimeSecret              = "HIVY_RUNTIME_SECRET"
	EmployeeEnvProxyAPIKey                = "HIVY_PROXY_API_KEY"
	EmployeeEnvAgentModel                 = "HIVY_AGENT_MODEL"
	EmployeeEnvAgentBaseURL               = "HIVY_AGENT_BASE_URL"
	EmployeeEnvAgentAPIKeyEnv             = "HIVY_AGENT_API_KEY_ENV"
	EmployeeEnvAgentMultimodalModel       = "HIVY_AGENT_MULTIMODAL_MODEL"
	EmployeeEnvAgentMultimodalBaseURL     = "HIVY_AGENT_MULTIMODAL_BASE_URL"
	EmployeeEnvAgentMultimodalAPIKeyEnv   = "HIVY_AGENT_MULTIMODAL_API_KEY_ENV"
	EmployeeEnvEmployeeID                 = "HIVY_EMPLOYEE_ID"
	EmployeeEnvCloudControlPlaneURL       = "HIVY_CONTROL_PLANE_URL"
	EmployeeEnvUploadBearer               = "HIVY_UPLOAD_BEARER"
	EmployeeEnvWorkspaceRoot              = "HIVY_WORKSPACE_ROOT"
	EmployeeEnvDBPath                     = "HIVY_DB_PATH"
	EmployeeEnvRuntimeBindAddr            = "HIVY_RUNTIME_BIND_ADDR"
	EmployeeEnvRuntimeMode                = "HIVY_RUNTIME_MODE"
	EmployeeEnvSandboxID                  = "HIVY_SANDBOX_ID"
	EmployeeEnvOrgID                      = "HIVY_ORG_ID"
	EmployeeEnvGitUsername                = "HIVY_GIT_USERNAME"
	EmployeeEnvGitEmail                   = "HIVY_GIT_EMAIL"
	EmployeeEnvGitCredentialsURL          = "HIVY_GIT_CREDENTIALS_URL"
	EmployeeEnvGitHubNoKeyring            = "GH_NO_KEYRING"
	EmployeeEnvDriveUploadURL             = "HIVY_DRIVE_UPLOAD_URL"
	EmployeeEnvBugsinkURL                 = "HIVY_BUGSINK_URL"
	EmployeeEnvBugsinkDashboardBaseURL    = "HIVY_BUGSINK_DASHBOARD_BASE_URL"
	EmployeeEnvBugsinkToken               = "HIVY_BUGSINK_TOKEN"
	EmployeeEnvLinearURL                  = "HIVY_LINEAR_URL"
	EmployeeEnvLinearToken                = "HIVY_LINEAR_TOKEN"
	EmployeeEnvNotionAPIURL               = "HIVY_NOTION_API_URL"
	EmployeeEnvNotionToken                = "HIVY_NOTION_TOKEN"
	EmployeeEnvSentryDSN                  = "SENTRY_DSN"
	EmployeeEnvSentryEnvironment          = "SENTRY_ENVIRONMENT"
	EmployeeEnvSentrySampleRate           = "SENTRY_SAMPLE_RATE"
	EmployeeEnvSentryTracesSampleRate     = "SENTRY_TRACES_SAMPLE_RATE"
	EmployeeEnvSentryEnableLogs           = "SENTRY_ENABLE_LOGS"
	EmployeeEnvSentryRelease              = "SENTRY_RELEASE"
	EmployeeEnvHome                       = "HOME"
	EmployeeEnvPath                       = "PATH"
	EmployeeEnvLang                       = "LANG"
	EmployeeEnvLCAll                      = "LC_ALL"
	EmployeeEnvSourceControlPlaneInjected = "control_plane_injected"
	EmployeeEnvSourceConditionalSentry    = "conditional_sentry"
	EmployeeEnvSourceContainerOS          = "container_os"
	EmployeeEnvStatusOK                   = "ok"
	EmployeeEnvStatusMissing              = "missing"
	EmployeeEnvStatusMissingOptional      = "missing_optional"
	EmployeeEnvStatusForbidden            = "forbidden"
	EmployeeEnvStatusUnexpected           = "unexpected"
	EmployeeEnvValueRedacted              = "<redacted>"
)

// ProxyAPIKeyEnv is kept for callers that already depend on this exported name.
const ProxyAPIKeyEnv = EmployeeEnvProxyAPIKey

type EmployeeEnvSpec struct {
	Key       string `json:"key"`
	Source    string `json:"source"`
	Sensitive bool   `json:"sensitive"`
	Forbidden bool   `json:"forbidden"`
	Optional  bool   `json:"optional"`
}

type EmployeeEnvReportEntry struct {
	Key       string `json:"key"`
	Source    string `json:"source"`
	Set       bool   `json:"set"`
	Sensitive bool   `json:"sensitive"`
	Forbidden bool   `json:"forbidden"`
	Status    string `json:"status"`
	Value     string `json:"value,omitempty"`
	Redacted  bool   `json:"redacted,omitempty"`
}

var employeeEnvCatalog = []EmployeeEnvSpec{
	{Key: EmployeeEnvRuntimeSecret, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvProxyAPIKey, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvAgentModel, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentBaseURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentAPIKeyEnv, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalModel, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalBaseURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalAPIKeyEnv, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvEmployeeID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvCloudControlPlaneURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvUploadBearer, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvWorkspaceRoot, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvDBPath, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvRuntimeBindAddr, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvRuntimeMode, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvSandboxID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvOrgID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitUsername, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitEmail, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitCredentialsURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitHubNoKeyring, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvDriveUploadURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvBugsinkURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvBugsinkDashboardBaseURL, Source: EmployeeEnvSourceControlPlaneInjected, Optional: true},
	{Key: EmployeeEnvBugsinkToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvLinearURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvLinearToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvNotionAPIURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvNotionToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvSentryDSN, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvSentryEnvironment, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvSentrySampleRate, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvSentryTracesSampleRate, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvSentryEnableLogs, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvSentryRelease, Source: EmployeeEnvSourceConditionalSentry, Optional: true},
	{Key: EmployeeEnvHome, Source: EmployeeEnvSourceContainerOS, Optional: true},
	{Key: EmployeeEnvPath, Source: EmployeeEnvSourceContainerOS, Optional: true},
	{Key: EmployeeEnvLang, Source: EmployeeEnvSourceContainerOS, Optional: true},
	{Key: EmployeeEnvLCAll, Source: EmployeeEnvSourceContainerOS, Optional: true},
}

func EmployeeEnvCatalog() []EmployeeEnvSpec {
	out := make([]EmployeeEnvSpec, len(employeeEnvCatalog))
	copy(out, employeeEnvCatalog)
	return out
}

func EmployeeForbiddenRawProviderEnvKeys() []string {
	out := make([]string, 0, 4)
	for _, spec := range employeeEnvCatalog {
		if spec.Forbidden {
			out = append(out, spec.Key)
		}
	}
	sort.Strings(out)
	return out
}

func EmployeeEnvReportFromKeys(keys []string, includeUnexpected bool) []EmployeeEnvReportEntry {
	env := make(map[string]string, len(keys))
	for _, key := range keys {
		if key != "" {
			env[key] = ""
		}
	}
	return EmployeeEnvReportFromEnv(env, includeUnexpected, false)
}

func EmployeeEnvReportFromEnv(env map[string]string, includeUnexpected bool, includeSensitive bool) []EmployeeEnvReportEntry {
	known := make(map[string]EmployeeEnvSpec, len(employeeEnvCatalog))
	out := make([]EmployeeEnvReportEntry, 0, len(employeeEnvCatalog))
	for _, spec := range employeeEnvCatalog {
		known[spec.Key] = spec
		value, present := env[spec.Key]
		status := EmployeeEnvStatusOK
		switch {
		case spec.Forbidden && present:
			status = EmployeeEnvStatusForbidden
		case spec.Forbidden:
			status = EmployeeEnvStatusOK
		case !present && spec.Optional:
			status = EmployeeEnvStatusMissingOptional
		case !present:
			status = EmployeeEnvStatusMissing
		}
		value, redacted := reportValue(value, present, spec.Sensitive, includeSensitive)
		out = append(out, EmployeeEnvReportEntry{
			Key:       spec.Key,
			Source:    spec.Source,
			Set:       present,
			Sensitive: spec.Sensitive,
			Forbidden: spec.Forbidden,
			Status:    status,
			Value:     value,
			Redacted:  redacted,
		})
	}
	if includeUnexpected {
		for key, value := range env {
			if _, ok := known[key]; ok {
				continue
			}
			sensitive := looksSensitiveEnvKey(key)
			value, redacted := reportValue(value, true, sensitive, includeSensitive)
			out = append(out, EmployeeEnvReportEntry{
				Key:       key,
				Source:    EmployeeEnvStatusUnexpected,
				Set:       true,
				Sensitive: sensitive,
				Status:    EmployeeEnvStatusUnexpected,
				Value:     value,
				Redacted:  redacted,
			})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Status == out[j].Status {
				return out[i].Key < out[j].Key
			}
			return out[i].Status < out[j].Status
		})
	}
	return out
}

func reportValue(value string, present bool, sensitive bool, includeSensitive bool) (string, bool) {
	if !present {
		return "", false
	}
	if sensitive && !includeSensitive {
		return EmployeeEnvValueRedacted, true
	}
	return value, false
}

func looksSensitiveEnvKey(key string) bool {
	key = strings.ToUpper(key)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTH", "BEARER"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
