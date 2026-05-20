package employeeruntime

import (
	"reflect"
	"testing"
)

func TestEmployeeEnvCatalogGolden(t *testing.T) {
	got := EmployeeEnvCatalog()
	var keys []string
	for _, spec := range got {
		keys = append(keys, spec.Key)
	}
	want := []string{
		EmployeeEnvRuntimeSecret,
		EmployeeEnvSlackBotToken,
		EmployeeEnvSlackAppToken,
		EmployeeEnvProxyAPIKey,
		EmployeeEnvAgentModel,
		EmployeeEnvAgentBaseURL,
		EmployeeEnvAgentAPIKeyEnv,
		EmployeeEnvAgentMultimodalModel,
		EmployeeEnvAgentMultimodalBaseURL,
		EmployeeEnvAgentMultimodalAPIKeyEnv,
		EmployeeEnvEmployeeID,
		EmployeeEnvCloudControlPlaneURL,
		EmployeeEnvBridgeAPIKey,
		EmployeeEnvUploadBearer,
		EmployeeEnvWorkspaceRoot,
		EmployeeEnvDBPath,
		EmployeeEnvRuntimeBindAddr,
		EmployeeEnvSandboxID,
		EmployeeEnvOrgID,
		EmployeeEnvHivyEmployeeID,
		EmployeeEnvGitUsername,
		EmployeeEnvGitEmail,
		EmployeeEnvGitCredentialsURL,
		EmployeeEnvGitHubNoKeyring,
		EmployeeEnvDriveUploadURL,
		EmployeeEnvBugsinkURL,
		EmployeeEnvBugsinkDashboardBaseURL,
		EmployeeEnvBugsinkToken,
		EmployeeEnvLinearURL,
		EmployeeEnvLinearToken,
		EmployeeEnvNotionAPIURL,
		EmployeeEnvNotionToken,
		EmployeeEnvSentryDSN,
		EmployeeEnvSentryEnvironment,
		EmployeeEnvSentrySampleRate,
		EmployeeEnvSentryTracesSampleRate,
		EmployeeEnvSentryEnableLogs,
		EmployeeEnvSentryRelease,
		EmployeeEnvHome,
		EmployeeEnvPath,
		EmployeeEnvLang,
		EmployeeEnvLCAll,
		EmployeeForbiddenEnvOpenRouterAPIKey,
		EmployeeForbiddenEnvOpenAIAPIKey,
		EmployeeForbiddenEnvGroqAPIKey,
		EmployeeForbiddenEnvTogetherAPIKey,
	}
	if !reflect.DeepEqual(gotKeys(got), want) {
		t.Fatalf("employee env catalog keys changed\ngot:  %#v\nwant: %#v", keys, want)
	}
}

func TestEmployeeEnvReport_TracksMissingForbiddenAndRedactedValues(t *testing.T) {
	report := EmployeeEnvReportFromEnv(map[string]string{
		EmployeeEnvRuntimeSecret:             "runtime-secret",
		EmployeeEnvAgentBaseURL:              "https://proxy.example.test/v1",
		EmployeeEnvProxyAPIKey:               "ptok_test",
		EmployeeForbiddenEnvOpenRouterAPIKey: "sk-or-test",
	}, false, false)
	byKey := map[string]EmployeeEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[EmployeeEnvRuntimeSecret]; !got.Set || got.Status != EmployeeEnvStatusOK || !got.Sensitive || got.Value != EmployeeEnvValueRedacted || !got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey[EmployeeEnvAgentBaseURL]; !got.Set || got.Value != "https://proxy.example.test/v1" || got.Redacted {
		t.Fatalf("non-sensitive env report = %+v", got)
	}
	if got := byKey[EmployeeEnvSlackBotToken]; got.Set || got.Status != EmployeeEnvStatusMissing || !got.Sensitive {
		t.Fatalf("missing slack token report = %+v", got)
	}
	if got := byKey[EmployeeForbiddenEnvOpenRouterAPIKey]; !got.Set || got.Status != EmployeeEnvStatusForbidden || !got.Forbidden {
		t.Fatalf("forbidden provider key report = %+v", got)
	}
	if got := byKey[EmployeeForbiddenEnvOpenAIAPIKey]; got.Set || got.Status != EmployeeEnvStatusOK || !got.Forbidden {
		t.Fatalf("absent forbidden provider key report = %+v", got)
	}
}

func TestEmployeeEnvReport_PrintsSensitiveValuesWhenRequested(t *testing.T) {
	report := EmployeeEnvReportFromEnv(map[string]string{
		EmployeeEnvRuntimeSecret: "runtime-secret",
		"EXTRA_API_TOKEN":        "extra-token",
	}, true, true)
	byKey := map[string]EmployeeEnvReportEntry{}
	for _, entry := range report {
		byKey[entry.Key] = entry
	}
	if got := byKey[EmployeeEnvRuntimeSecret]; got.Value != "runtime-secret" || got.Redacted {
		t.Fatalf("runtime secret report = %+v", got)
	}
	if got := byKey["EXTRA_API_TOKEN"]; !got.Sensitive || got.Value != "extra-token" || got.Redacted {
		t.Fatalf("unexpected sensitive env report = %+v", got)
	}
}

func gotKeys(specs []EmployeeEnvSpec) []string {
	keys := make([]string, 0, len(specs))
	for _, spec := range specs {
		keys = append(keys, spec.Key)
	}
	return keys
}
