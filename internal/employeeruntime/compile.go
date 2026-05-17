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
	"github.com/usehiveloop/hiveloop/internal/employeeprompts"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/token"
)

const (
	DefaultEmployeeModel           = "deepseek/deepseek-v4-flash"
	DefaultEmployeeMultimodalModel = "google/gemini-3-flash-preview"
	proxyTokenTTL                  = 24 * time.Hour
)

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
	Hindsight  HindsightRecallClient
}

type HindsightRecallClient interface {
	Recall(ctx context.Context, bankID string, req *hindsight.RecallRequest) (*hindsight.RecallResponse, error)
}

type StartupSecrets struct {
	SlackBotToken string
	SlackAppToken string
	ProxyToken    string
}

type AgentDefinition struct {
	Agent            AgentMeta        `json:"agent"`
	PromptFragments  PromptFragments  `json:"prompt_fragments,omitempty"`
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
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PromptFragments struct {
	Identity            PromptFragment `json:"identity,omitempty"`
	Company             PromptFragment `json:"company,omitempty"`
	Team                PromptFragment `json:"team,omitempty"`
	OperatingPrinciples PromptFragment `json:"operating_principles,omitempty"`
}

type PromptFragment struct {
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
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

type SlackConfig struct {
	PostableChannels []SlackChannelSpec `json:"postable_channels,omitempty"`
}

type SlackChannelSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPrivate   bool   `json:"is_private,omitempty"`
}

type MemoryContext struct {
	Entries     []MemoryContextEntry `json:"entries"`
	TokenBudget int                  `json:"token_budget"`
}

type MemoryContextEntry struct {
	Content    string   `json:"content"`
	Source     string   `json:"source,omitempty"`
	MemoryType string   `json:"memory_type,omitempty"`
	Tags       []string `json:"tags,omitempty"`
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
	if ourMCP := buildEmployeeMCPServer(ctx, deps, agent); ourMCP != nil {
		mcpServers = upsertHiveloopMCPServer(mcpServers, ourMCP)
	}
	description := ""
	if agent.Description != nil {
		description = *agent.Description
	}
	fragments := buildPromptFragments(ctx, deps.DB, agent, description)
	contextMap := map[string]any{
		"memory": buildMemoryContext(ctx, deps, agent),
	}
	slackConfig := buildSlackConfig(ctx, deps, agent)
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:        agent.Name,
			Description: description,
		},
		PromptFragments:  fragments,
		Model:            proxyModel(deps.Cfg, DefaultEmployeeModel),
		MultimodalModel:  ptrModel(proxyModel(deps.Cfg, DefaultEmployeeMultimodalModel)),
		Limits:           defaultLimits(),
		Context:          contextMap,
		Tools:            defaultTools(),
		McpServers:       mcpServers,
		Skills:           skills,
		Subagents:        []any{},
		Slack:            slackConfigMap(slackConfig),
		OutboundChannels: []any{},
	}, nil
}

func ControlPlaneOutboundChannels(cfg *config.Config, sandboxID uuid.UUID) []any {
	bridgeHost := "api.usehiveloop.com"
	if cfg != nil && strings.TrimSpace(cfg.BridgeHost) != "" {
		bridgeHost = strings.TrimRight(strings.TrimSpace(cfg.BridgeHost), "/")
	}
	return []any{
		map[string]any{
			"name":       "control-plane-memory",
			"type":       "webhook",
			"url":        fmt.Sprintf("https://%s/internal/webhooks/employee/%s", bridgeHost, sandboxID),
			"secret_env": EmployeeEnvRuntimeSecret,
		},
	}
}

func buildMemoryContext(ctx context.Context, deps CompileDeps, agent *model.Agent) MemoryContext {
	memory := MemoryContext{Entries: []MemoryContextEntry{}, TokenBudget: 1000}
	if deps.Hindsight == nil || agent == nil || agent.OrgID == nil {
		return memory
	}
	recallCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := "Durable company, team, people, project, decision, policy, preference, technical, customer, and communication-behavior memories relevant to this employee's current work."
	result, err := deps.Hindsight.Recall(recallCtx, hindsight.OrgBankID(*agent.OrgID), &hindsight.RecallRequest{
		Query:     query,
		Budget:    "mid",
		TagGroups: employeeMemoryTagGroups(agent),
	})
	if err != nil || result == nil {
		return memory
	}
	memory.Entries = compactMemoryResults(result.Results, 12, memory.TokenBudget)
	return memory
}

func employeeMemoryTagGroups(agent *model.Agent) []any {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	tags := []string{"company:" + agent.OrgID.String()}
	if agent.TeamID != nil {
		tags = append(tags, "team:"+agent.TeamID.String())
	} else if strings.TrimSpace(agent.Team) != "" {
		tags = append(tags, "team:"+strings.TrimSpace(agent.Team))
	}
	return []any{map[string]any{"tags": tags, "match": "all_strict"}}
}

func compactMemoryResults(results []any, maxEntries int, tokenBudget int) []MemoryContextEntry {
	entries := make([]MemoryContextEntry, 0, len(results))
	remainingChars := tokenBudget * 4
	for _, raw := range results {
		if len(entries) >= maxEntries || remainingChars <= 0 {
			break
		}
		entry := memoryEntryFromResult(raw)
		entry.Content = strings.TrimSpace(entry.Content)
		if entry.Content == "" {
			continue
		}
		if len(entry.Content) > remainingChars {
			entry.Content = entry.Content[:remainingChars]
		}
		remainingChars -= len(entry.Content)
		entries = append(entries, entry)
	}
	return entries
}

func memoryEntryFromResult(raw any) MemoryContextEntry {
	switch value := raw.(type) {
	case string:
		return MemoryContextEntry{Content: value}
	case map[string]any:
		return memoryEntryFromMap(value)
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return MemoryContextEntry{}
		}
		var m map[string]any
		if err := json.Unmarshal(bytes, &m); err != nil {
			return MemoryContextEntry{Content: string(bytes)}
		}
		return memoryEntryFromMap(m)
	}
}

func memoryEntryFromMap(m map[string]any) MemoryContextEntry {
	entry := MemoryContextEntry{
		Content:    firstString(m, "content", "text", "memory", "summary", "fact", "observation"),
		Source:     firstString(m, "source"),
		MemoryType: firstString(m, "memory_type", "type"),
	}
	if entry.Content == "" {
		if nested, ok := m["document"].(map[string]any); ok {
			entry.Content = firstString(nested, "content", "text", "summary")
		}
	}
	if tags, ok := m["tags"].([]any); ok {
		for _, raw := range tags {
			if tag, ok := raw.(string); ok {
				entry.Tags = append(entry.Tags, tag)
			}
		}
	}
	return entry
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildPromptFragments(ctx context.Context, db *gorm.DB, agent *model.Agent, description string) PromptFragments {
	var org model.Org
	var hasOrg bool
	var team model.Team
	var hasTeam bool
	if agent.OrgID != nil && db != nil {
		if err := db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err == nil {
			hasOrg = true
		}
	}
	if agent.TeamID != nil && db != nil {
		if err := db.WithContext(ctx).Where("id = ?", *agent.TeamID).First(&team).Error; err == nil {
			hasTeam = true
		}
	}

	fragments := PromptFragments{
		Identity: PromptFragment{
			Title: "Your identity",
			Content: strings.TrimSpace(strings.Join([]string{
				identityOpening(agent, org, hasOrg, team, hasTeam),
				"Name: " + agent.Name,
				optionalLine("Role description", description),
				employeeIdentityPrompt(agent),
			}, "\n")),
		},
	}
	if hasOrg {
		companyContent := strings.TrimSpace(org.PromptCompany)
		if companyContent == "" {
			companyContent = defaultCompanyPrompt(org)
		}
		if companyContent != "" {
			fragments.Company = PromptFragment{Title: "About the company", Content: companyContent}
		}
	}
	if fragments.Team.Content == "" && hasTeam {
		teamContent := strings.TrimSpace(team.PromptTeam)
		if teamContent == "" {
			teamContent = defaultTeamPrompt(team)
		}
		if teamContent != "" {
			fragments.Team = PromptFragment{
				Title:   "About your team",
				Content: teamContent,
			}
		}
	}
	if strings.TrimSpace(agent.PromptOperatingPrinciples) != "" {
		fragments.OperatingPrinciples = PromptFragment{
			Title:   "Operating principles",
			Content: strings.TrimSpace(agent.PromptOperatingPrinciples),
		}
	}
	return fragments
}

func identityOpening(agent *model.Agent, org model.Org, hasOrg bool, team model.Team, hasTeam bool) string {
	companyName := "this company"
	if hasOrg && strings.TrimSpace(org.Name) != "" {
		companyName = strings.TrimSpace(org.Name)
	}
	teamName := strings.TrimSpace(agent.Team)
	if teamName == "" && hasTeam {
		teamName = strings.TrimSpace(team.Name)
	}
	if teamName == "" {
		teamName = "your"
	}
	if teamName == "your" {
		return fmt.Sprintf("You are a %s employee working on your team.", companyName)
	}
	return fmt.Sprintf("You are a %s employee working on the %s team.", companyName, teamName)
}

func employeeIdentityPrompt(agent *model.Agent) string {
	if agent.IdentityPrompt != "" {
		return strings.TrimSpace(agent.IdentityPrompt)
	}
	if agent.Category != nil && strings.EqualFold(strings.TrimSpace(*agent.Category), "engineering") {
		return employeeprompts.EngineeringIdentityPrompt
	}
	return employeeprompts.EngineeringIdentityPrompt
}

func optionalLine(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func defaultCompanyPrompt(org model.Org) string {
	var parts []string
	if org.Name != "" {
		parts = append(parts, "Company name: "+org.Name)
	}
	if org.Website != "" {
		parts = append(parts, "Website: "+org.Website)
	}
	if org.Description != "" {
		parts = append(parts, "Company description: "+org.Description)
	}
	return strings.Join(parts, "\n")
}

func defaultTeamPrompt(team model.Team) string {
	var parts []string
	if team.Name != "" {
		parts = append(parts, "Team: "+team.Name)
	}
	if team.Description != "" {
		parts = append(parts, "Team description: "+team.Description)
	}
	return strings.Join(parts, "\n")
}

func buildEmployeeMCPServer(ctx context.Context, deps CompileDeps, agent *model.Agent) any {
	if deps.DB == nil || deps.Cfg == nil || deps.Cfg.MCPBaseURL == "" || agent.OrgID == nil {
		return nil
	}
	var tok model.Token
	if err := deps.DB.WithContext(ctx).
		Where("org_id = ? AND expires_at > ? AND meta->>'agent_id' = ? AND meta->>'type' = ?", *agent.OrgID, time.Now(), agent.ID.String(), "agent_proxy").
		Order("created_at DESC").
		First(&tok).Error; err != nil {
		return nil
	}
	url := fmt.Sprintf("%s/%s", strings.TrimRight(deps.Cfg.MCPBaseURL, "/"), tok.JTI)
	return map[string]any{
		"name":      "hiveloop",
		"transport": "streamable_http",
		"url":       url,
		"headers": map[string]string{
			"Authorization": employeeMCPAuthorizationHeader(),
		},
		"default_enable_all_tools": true,
	}
}

func employeeMCPAuthorizationHeader() string {
	return "Bearer ${" + ProxyAPIKeyEnv + "}"
}

func upsertHiveloopMCPServer(servers []any, server any) []any {
	out := make([]any, 0, len(servers)+1)
	for _, existing := range servers {
		if m, ok := existing.(map[string]any); ok {
			if name, _ := m["name"].(string); name == "hiveloop" {
				continue
			}
		}
		out = append(out, existing)
	}
	return append(out, server)
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
		{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 60, "max_output_bytes": 5 * 1024 * 1024, "deny_patterns": []string{"rm -rf /", "rm -rf ~", "mkfs", "dd if=", ":(){:|:&};:", "shutdown", "reboot"}, "env_passthrough": []string{EmployeeEnvHome, EmployeeEnvPath, EmployeeEnvLang, EmployeeEnvLCAll, ProxyAPIKeyEnv, EmployeeEnvBugsinkURL, EmployeeEnvBugsinkDashboardBaseURL, EmployeeEnvBugsinkToken, EmployeeEnvLinearURL, EmployeeEnvLinearToken}, "sandbox": "process_isolated"}},
		{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{}}},
		{"type": "builtin.write_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 5 * 1024 * 1024, "deny_globs": []string{}, "atomic": true}},
		{"type": "builtin.post_status_update"}, {"type": "builtin.post_to_slack_channel"},
		{"type": "builtin.cron"}, {"type": "builtin.delegate"}, {"type": "builtin.check_delegated_status"},
		{"type": "builtin.check_bash_status"}, {"type": "builtin.wake"}, {"type": "builtin.load_tools"},
		{"type": "builtin.skills_list"}, {"type": "builtin.skill_view"}, {"type": "builtin.skill_manage"},
		{"type": "builtin.cloud_agent_launch_task"}, {"type": "builtin.cloud_agent_task_status"}, {"type": "builtin.cloud_agent_list_tasks"},
		{"type": "builtin.cloud_agent_task_send_message"}, {"type": "builtin.cloud_agent_task_terminate"},
	}
}

func buildSlackConfig(ctx context.Context, deps CompileDeps, agent *model.Agent) SlackConfig {
	cfg := SlackConfig{PostableChannels: []SlackChannelSpec{}}
	if deps.DB == nil || deps.KMS == nil || agent == nil {
		return cfg
	}
	profile, err := loadActiveSlackProfile(ctx, deps.DB, agent.ID)
	if err != nil {
		return cfg
	}
	secrets, err := decryptSlackSecrets(ctx, deps.KMS, profile)
	if err != nil {
		return cfg
	}
	channelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	channels, err := slackprov.ListBotChannels(channelCtx, secrets.BotToken)
	if err != nil {
		return cfg
	}
	cfg.PostableChannels = slackChannelSpecs(channels)
	return cfg
}

func slackChannelSpecs(channels []slackprov.Channel) []SlackChannelSpec {
	out := make([]SlackChannelSpec, 0, len(channels))
	for _, ch := range channels {
		description := strings.TrimSpace(ch.Topic)
		if description == "" {
			description = strings.TrimSpace(ch.Purpose)
		} else if purpose := strings.TrimSpace(ch.Purpose); purpose != "" && purpose != description {
			description += " " + purpose
		}
		out = append(out, SlackChannelSpec{
			ID:          ch.ID,
			Name:        ch.Name,
			Description: description,
			IsPrivate:   ch.IsPrivate,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func slackConfigMap(cfg SlackConfig) map[string]any {
	out := map[string]any{}
	if len(cfg.PostableChannels) == 0 {
		return out
	}
	channels := make([]map[string]any, 0, len(cfg.PostableChannels))
	for _, ch := range cfg.PostableChannels {
		item := map[string]any{
			"id":   ch.ID,
			"name": ch.Name,
		}
		if ch.Description != "" {
			item["description"] = ch.Description
		}
		if ch.IsPrivate {
			item["is_private"] = true
		}
		channels = append(channels, item)
	}
	out["postable_channels"] = channels
	return out
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
	Description                  string            `json:"description"`
	Content                      string            `json:"content"`
	Files                        map[string]string `json:"files"`
	Manifest                     map[string]any    `json:"manifest"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
}

func buildSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]SkillSpec, error) {
	if db == nil {
		return []SkillSpec{}, nil
	}
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
		requiredEnv := normalizeRequiredEnvironmentVariables(bundle.RequiredEnvironmentVariables)
		if len(requiredEnv) == 0 {
			requiredEnv = requiredEnvironmentVariablesFromManifest(bundle.Manifest)
		}
		out = append(out, SkillSpec{
			Name:                         skill.Slug,
			Description:                  description,
			Trigger:                      map[string]any{"type": "keyword", "patterns": []string{skill.Slug, skill.Name}},
			Instructions:                 composeInstructions(skill, bundle),
			Files:                        bundle.Files,
			Tags:                         tags,
			RelatedSkills:                []string{},
			RequiredEnvironmentVariables: requiredEnv,
			RequiredCredentialFiles:      []string{},
		})
	}
	return out, nil
}

func normalizeRequiredEnvironmentVariables(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func requiredEnvironmentVariablesFromManifest(manifest map[string]any) []string {
	if len(manifest) == 0 {
		return []string{}
	}
	raw, ok := manifest["required_environment_variables"]
	if !ok {
		return []string{}
	}
	switch value := raw.(type) {
	case []string:
		return normalizeRequiredEnvironmentVariables(value)
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
		return normalizeRequiredEnvironmentVariables(values)
	default:
		return []string{}
	}
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
