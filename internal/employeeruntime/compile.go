package employeeruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/token"
)

const (
	DefaultEmployeeModel           = "deepseek/deepseek-v4-flash"
	DefaultEmployeeMultimodalModel = "google/gemini-3-flash-preview"
	ProxyAPIKeyEnv                 = "HIVELOOP_PROXY_API_KEY"
	proxyTokenTTL                  = 24 * time.Hour
)

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
}

type StartupSecrets struct {
	SlackBotToken string
	SlackAppToken string
	ProxyToken    string
}

type AgentDefinition struct {
	Agent            AgentMeta        `json:"agent"`
	Model            ModelConfig      `json:"model"`
	MultimodalModel  *ModelConfig     `json:"multimodal_model,omitempty"`
	Limits           map[string]any   `json:"limits,omitempty"`
	Context          map[string]any   `json:"context,omitempty"`
	Tools            []map[string]any `json:"tools"`
	McpServers       []any            `json:"mcp_servers"`
	Skills           []SkillSpec      `json:"skills"`
	Subagents        []any            `json:"subagents"`
	Slack            map[string]any   `json:"slack,omitempty"`
	OutboundChannels []any            `json:"outbound_channels"`
}

type AgentMeta struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
}

type ModelConfig struct {
	Provider        string            `json:"provider"`
	BaseURL         string            `json:"base_url"`
	ModelID         string            `json:"model_id"`
	APIKeyEnv       string            `json:"api_key_env"`
	Temperature     *float64          `json:"temperature,omitempty"`
	MaxOutputTokens *uint32           `json:"max_output_tokens,omitempty"`
	ReasoningEffort *string           `json:"reasoning_effort,omitempty"`
	ExtraHeaders    map[string]string `json:"extra_headers"`
	Fallback        *ModelConfig      `json:"fallback,omitempty"`
}

type SkillSpec struct {
	Name                         string            `json:"name"`
	Description                  string            `json:"description"`
	Trigger                      map[string]any    `json:"trigger"`
	Instructions                 string            `json:"instructions"`
	Files                        map[string]string `json:"files,omitempty"`
	Category                     *string           `json:"category,omitempty"`
	Tags                         []string          `json:"tags"`
	RelatedSkills                []string          `json:"related_skills"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
	RequiredCredentialFiles      []string          `json:"required_credential_files"`
	Pinned                       bool              `json:"pinned"`
}

func PrepareStartup(ctx context.Context, deps CompileDeps, agent *model.Agent) (*StartupSecrets, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("employee runtime startup: agent must have org_id")
	}
	profile, err := loadActiveSlackProfile(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, err
	}
	slack, err := decryptSlackSecrets(ctx, deps.KMS, profile)
	if err != nil {
		return nil, err
	}
	cred, err := credentials.Resolve(ctx, deps.DB, deps.Picker, agent)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}
	proxyToken, jti, err := token.Mint(
		deps.SigningKey,
		agent.OrgID.String(),
		cred.ID.String(),
		proxyTokenTTL,
		token.MintOptions{IsSystem: cred.IsSystem},
	)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	now := time.Now()
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    now.Add(proxyTokenTTL),
		Scopes:       scopesFromIntegrations(agent.Integrations),
		Meta:         model.JSON{"agent_id": agent.ID.String(), "type": "agent_proxy", "harness": "employee-sandbox"},
	}
	if err := deps.DB.WithContext(ctx).Create(&dbToken).Error; err != nil {
		return nil, fmt.Errorf("persist proxy token: %w", err)
	}
	return &StartupSecrets{
		SlackBotToken: slack.BotToken,
		SlackAppToken: slack.AppToken,
		ProxyToken:    "ptok_" + proxyToken,
	}, nil
}

func Compile(ctx context.Context, deps CompileDeps, agent *model.Agent) (*AgentDefinition, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("employee runtime compile: agent must have org_id")
	}
	skills, err := buildSkills(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, err
	}
	mcpServers := jsonArray(agent.McpServers)
	description := ""
	if agent.Description != nil {
		description = *agent.Description
	}
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:         agent.Name,
			Description:  description,
			SystemPrompt: agent.SystemPrompt,
		},
		Model:            proxyModel(deps.Cfg, DefaultEmployeeModel),
		MultimodalModel:  ptrModel(proxyModel(deps.Cfg, DefaultEmployeeMultimodalModel)),
		Limits:           defaultLimits(),
		Context:          map[string]any{},
		Tools:            defaultTools(),
		McpServers:       mcpServers,
		Skills:           skills,
		Subagents:        []any{},
		Slack:            map[string]any{},
		OutboundChannels: []any{},
	}, nil
}

func proxyModel(cfg *config.Config, modelID string) ModelConfig {
	baseURL := "https://proxy.hiveloop.com/v1"
	if cfg != nil && cfg.ProxyHost != "" {
		baseURL = "https://" + strings.TrimRight(cfg.ProxyHost, "/") + "/v1"
	}
	temp := 0.3
	maxOutput := uint32(8192)
	reasoning := "low"
	return ModelConfig{
		Provider:        "openai_compatible",
		BaseURL:         baseURL,
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
		{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 60, "max_output_bytes": 5 * 1024 * 1024, "deny_patterns": []string{"rm -rf /", "rm -rf ~", "mkfs", "dd if=", ":(){:|:&};:", "shutdown", "reboot"}, "env_passthrough": []string{"HOME", "PATH", "LANG", "LC_ALL", ProxyAPIKeyEnv}, "sandbox": "process_isolated"}},
		{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{"**/.env", "**/.env.*", "**/secrets/**", "**/id_rsa*", "**/*.pem"}}},
		{"type": "builtin.write_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{"**/.env", "**/.env.*", "**/secrets/**", "**/.git/**", "**/node_modules/**"}, "atomic": true}},
		{"type": "builtin.post_status_update"}, {"type": "builtin.post_to_channel"},
		{"type": "builtin.cron"}, {"type": "builtin.delegate"}, {"type": "builtin.check_delegated_status"},
		{"type": "builtin.check_bash_status"}, {"type": "builtin.wake"}, {"type": "builtin.load_tools"},
		{"type": "builtin.skills_list"}, {"type": "builtin.skill_view"}, {"type": "builtin.skill_manage"},
		{"type": "builtin.cloud_agent_launch_task"}, {"type": "builtin.cloud_agent_task_status"}, {"type": "builtin.cloud_agent_list_tasks"},
		{"type": "builtin.cloud_agent_task_send_message"}, {"type": "builtin.cloud_agent_task_terminate"},
	}
}

func loadActiveSlackProfile(ctx context.Context, db *gorm.DB, agentID uuid.UUID) (*model.AgentProfile, error) {
	var profile model.AgentProfile
	err := db.WithContext(ctx).
		Where("agent_id = ? AND provider = ? AND status = ? AND deleted_at IS NULL", agentID, slackprov.Provider, "active").
		First(&profile).Error
	if err != nil {
		return nil, fmt.Errorf("active slack profile required for employee sandbox startup: %w", err)
	}
	return &profile, nil
}

func decryptSlackSecrets(ctx context.Context, kms *crypto.KeyWrapper, profile *model.AgentProfile) (*slackprov.Secrets, error) {
	if kms == nil || len(profile.EncryptedSecrets) == 0 || len(profile.WrappedDEK) == 0 {
		return nil, fmt.Errorf("slack profile has no encrypted secrets")
	}
	dek, err := kms.Unwrap(ctx, profile.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("unwrap slack DEK: %w", err)
	}
	defer wipe(dek)
	plaintext, err := crypto.DecryptCredential(profile.EncryptedSecrets, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt slack secrets: %w", err)
	}
	var secrets slackprov.Secrets
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("parse slack secrets: %w", err)
	}
	if secrets.BotToken == "" || secrets.AppToken == "" {
		return nil, fmt.Errorf("slack bot token and app token are required")
	}
	return &secrets, nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
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

type skillBundle struct {
	Description string            `json:"description"`
	Content     string            `json:"content"`
	Files       map[string]string `json:"files"`
}

func buildSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]SkillSpec, error) {
	var links []model.AgentSkill
	if err := db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&links).Error; err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return []SkillSpec{}, nil
	}
	ids := make([]uuid.UUID, 0, len(links))
	pinnedBySkill := make(map[uuid.UUID]*uuid.UUID, len(links))
	for _, link := range links {
		ids = append(ids, link.SkillID)
		pinnedBySkill[link.SkillID] = link.PinnedVersionID
	}
	var skills []model.Skill
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&skills).Error; err != nil {
		return nil, err
	}
	versionIDs := make([]uuid.UUID, 0, len(skills))
	for _, skill := range skills {
		if pinned := pinnedBySkill[skill.ID]; pinned != nil {
			versionIDs = append(versionIDs, *pinned)
		} else if skill.LatestVersionID != nil {
			versionIDs = append(versionIDs, *skill.LatestVersionID)
		}
	}
	if len(versionIDs) == 0 {
		return []SkillSpec{}, nil
	}
	var versions []model.SkillVersion
	if err := db.WithContext(ctx).Where("id IN ?", versionIDs).Find(&versions).Error; err != nil {
		return nil, err
	}
	versionsByID := make(map[uuid.UUID]model.SkillVersion, len(versions))
	for _, version := range versions {
		versionsByID[version.ID] = version
	}
	out := make([]SkillSpec, 0, len(skills))
	for _, skill := range skills {
		var versionID uuid.UUID
		if pinned := pinnedBySkill[skill.ID]; pinned != nil {
			versionID = *pinned
		} else if skill.LatestVersionID != nil {
			versionID = *skill.LatestVersionID
		} else {
			continue
		}
		version, ok := versionsByID[versionID]
		if !ok {
			continue
		}
		var bundle skillBundle
		if err := json.Unmarshal(version.Bundle, &bundle); err != nil {
			continue
		}
		description := bundle.Description
		if skill.Description != nil && *skill.Description != "" {
			description = *skill.Description
		}
		tags := []string(skill.Tags)
		sort.Strings(tags)
		out = append(out, SkillSpec{
			Name:         skill.Slug,
			Description:  description,
			Trigger:      map[string]any{"type": "keyword", "patterns": []string{skill.Slug, skill.Name}},
			Instructions: composeInstructions(skill, bundle),
			Files:        bundle.Files,
			Tags:         tags,
		})
	}
	return out, nil
}

func composeInstructions(skill model.Skill, bundle skillBundle) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(skill.Slug)
	b.WriteString("\n")
	if bundle.Description != "" {
		b.WriteString("description: ")
		encoded, _ := json.Marshal(bundle.Description)
		b.Write(encoded)
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(bundle.Content)
	if !strings.HasSuffix(bundle.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
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
