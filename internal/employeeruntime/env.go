package employeeruntime

import (
	"sort"
	"strings"
)

const (
	EmployeeEnvRuntimeSecret              = "RUNTIME_SECRET"
	EmployeeEnvSlackBotToken              = "SLACK_BOT_TOKEN"
	EmployeeEnvSlackAppToken              = "SLACK_APP_TOKEN"
	EmployeeEnvProxyAPIKey                = "HIVELOOP_PROXY_API_KEY"
	EmployeeEnvAgentModel                 = "AGENT_MODEL"
	EmployeeEnvAgentBaseURL               = "AGENT_BASE_URL"
	EmployeeEnvAgentAPIKeyEnv             = "AGENT_API_KEY_ENV"
	EmployeeEnvAgentMultimodalModel       = "AGENT_MULTIMODAL_MODEL"
	EmployeeEnvAgentMultimodalBaseURL     = "AGENT_MULTIMODAL_BASE_URL"
	EmployeeEnvAgentMultimodalAPIKeyEnv   = "AGENT_MULTIMODAL_API_KEY_ENV"
	EmployeeEnvEmployeeID                 = "EMPLOYEE_ID"
	EmployeeEnvCloudControlPlaneURL       = "CLOUD_CONTROL_PLANE_URL"
	EmployeeEnvBridgeAPIKey               = "BRIDGE_API_KEY"
	EmployeeEnvUploadBearer               = "UPLOAD_BEARER"
	EmployeeEnvWorkspaceRoot              = "WORKSPACE_ROOT"
	EmployeeEnvDBPath                     = "DB_PATH"
	EmployeeEnvRuntimeBindAddr            = "RUNTIME_BIND_ADDR"
	EmployeeEnvSandboxID                  = "HIVELOOP_SANDBOX_ID"
	EmployeeEnvOrgID                      = "HIVELOOP_ORG_ID"
	EmployeeEnvAgentID                    = "HIVELOOP_AGENT_ID"
	EmployeeEnvGitUsername                = "HIVELOOP_GIT_USERNAME"
	EmployeeEnvGitEmail                   = "HIVELOOP_GIT_EMAIL"
	EmployeeEnvGitCredentialsURL          = "HIVELOOP_GIT_CREDENTIALS_URL"
	EmployeeEnvGitHubNoKeyring            = "GH_NO_KEYRING"
	EmployeeEnvDriveUploadURL             = "HIVELOOP_DRIVE_UPLOAD_URL"
	EmployeeEnvBugsinkURL                 = "BUGSINK_URL"
	EmployeeEnvBugsinkToken               = "BUGSINK_TOKEN"
	EmployeeEnvLinearURL                  = "LINEAR_URL"
	EmployeeEnvLinearToken                = "LINEAR_TOKEN"
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
	EmployeeForbiddenEnvOpenRouterAPIKey  = "OPENROUTER_API_KEY"
	EmployeeForbiddenEnvOpenAIAPIKey      = "OPENAI_API_KEY"
	EmployeeForbiddenEnvGroqAPIKey        = "GROQ_API_KEY"
	EmployeeForbiddenEnvTogetherAPIKey    = "TOGETHER_API_KEY"
	EmployeeEnvSourceControlPlaneInjected = "control_plane_injected"
	EmployeeEnvSourceConditionalSentry    = "conditional_sentry"
	EmployeeEnvSourceContainerOS          = "container_os"
	EmployeeEnvSourceForbiddenRawProvider = "forbidden_raw_provider"
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
	{Key: EmployeeEnvSlackBotToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvSlackAppToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvProxyAPIKey, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvAgentModel, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentBaseURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentAPIKeyEnv, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalModel, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalBaseURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentMultimodalAPIKeyEnv, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvEmployeeID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvCloudControlPlaneURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvBridgeAPIKey, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvUploadBearer, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvWorkspaceRoot, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvDBPath, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvRuntimeBindAddr, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvSandboxID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvOrgID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvAgentID, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitUsername, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitEmail, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitCredentialsURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvGitHubNoKeyring, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvDriveUploadURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvBugsinkURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvBugsinkToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
	{Key: EmployeeEnvLinearURL, Source: EmployeeEnvSourceControlPlaneInjected},
	{Key: EmployeeEnvLinearToken, Source: EmployeeEnvSourceControlPlaneInjected, Sensitive: true},
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
	{Key: EmployeeForbiddenEnvOpenRouterAPIKey, Source: EmployeeEnvSourceForbiddenRawProvider, Sensitive: true, Forbidden: true, Optional: true},
	{Key: EmployeeForbiddenEnvOpenAIAPIKey, Source: EmployeeEnvSourceForbiddenRawProvider, Sensitive: true, Forbidden: true, Optional: true},
	{Key: EmployeeForbiddenEnvGroqAPIKey, Source: EmployeeEnvSourceForbiddenRawProvider, Sensitive: true, Forbidden: true, Optional: true},
	{Key: EmployeeForbiddenEnvTogetherAPIKey, Source: EmployeeEnvSourceForbiddenRawProvider, Sensitive: true, Forbidden: true, Optional: true},
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
