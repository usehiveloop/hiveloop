package employeeruntime

import (
	"encoding/json"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func proxyModel(cfg *config.Config, modelID string) ModelConfig {
	temp := 0.3
	maxOutput := uint32(8192)
	reasoning := "low"
	return ModelConfig{
		Provider:        "openai_compatible",
		BaseURL:         cfg.ProxyOpenAIBaseURL(),
		ModelID:         modelID,
		APIKeyEnv:       ProxyAPIKeyEnv,
		Temperature:     &temp,
		MaxOutputTokens: &maxOutput,
		ReasoningEffort: &reasoning,
		ExtraHeaders:    map[string]string{},
	}
}

func ptrModel(m ModelConfig) *ModelConfig { return &m }

func defaultLimits() map[string]any {
	return map[string]any{
		"max_turns_per_session":     50,
		"input_token_budget":        180000,
		"output_token_budget":       8000,
		"tool_call_timeout_seconds": 60,
		"subagent_max_depth":        2,
	}
}

func defaultTools() []map[string]any {
	return []map[string]any{
		{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 60, "max_output_bytes": 5 * 1024 * 1024, "deny_patterns": []string{"rm -rf /", "rm -rf ~", "mkfs", "dd if=", ":(){:|:&};:", "shutdown", "reboot"}, "env_passthrough": []string{EmployeeEnvHome, EmployeeEnvPath, EmployeeEnvLang, EmployeeEnvLCAll, ProxyAPIKeyEnv, EmployeeEnvBugsinkURL, EmployeeEnvBugsinkDashboardBaseURL, EmployeeEnvBugsinkToken, EmployeeEnvLinearURL, EmployeeEnvLinearToken, EmployeeEnvNotionAPIURL, EmployeeEnvNotionToken}, "sandbox": "process_isolated"}},
		{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{}}},
		{"type": "builtin.write_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{}, "atomic": true}},
		{"type": "builtin.cron"}, {"type": "builtin.delegate"}, {"type": "builtin.check_delegated_status"},
		{"type": "builtin.check_bash_status"}, {"type": "builtin.wake"}, {"type": "builtin.load_tools"},
		{"type": "builtin.skills_list"}, {"type": "builtin.skill_view"}, {"type": "builtin.skill_manage"},
	}
}

func jsonArray(raw model.JSON) []any {
	if len(raw) == 0 {
		return []any{}
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return []any{}
	}
	var arr []any
	if err := json.Unmarshal(bytes, &arr); err != nil {
		return []any{}
	}
	return arr
}
