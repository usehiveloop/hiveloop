package employeeruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/token"
)

const (
	DefaultEmployeeModel           = "deepseek/deepseek-v4-flash"
	DefaultEmployeeSubagentModel   = "deepseek/deepseek-v4-pro"
	DefaultEmployeeMultimodalModel = "google/gemini-3-flash-preview"
	proxyTokenTTL                  = 24 * time.Hour
	managedEmployeeName            = "Hivy"
	managedEmployeeDescription     = "Hivy is the organization's managed AI employee."
)

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
	Nango      *nango.Client
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

type ProxyTokenResult struct {
	Token     string
	JTI       string
	ExpiresAt time.Time
}

type EmployeeDefinition struct {
	Employee         AgentMeta        `json:"employee"`
	PromptFragments  PromptFragments  `json:"prompt_fragments,omitempty"`
	Model            ModelConfig      `json:"model"`
	MultimodalModel  *ModelConfig     `json:"multimodal_model,omitempty"`
	Limits           map[string]any   `json:"limits,omitempty"`
	Context          map[string]any   `json:"context,omitempty"`
	Tools            []map[string]any `json:"tools"`
	McpServers       []any            `json:"mcp_servers"`
	Skills           []SkillSpec      `json:"skills"`
	Specialists      []any            `json:"specialists"`
	Slack            map[string]any   `json:"slack,omitempty"`
	OutboundChannels []any            `json:"outbound_channels"`
}

type AgentDefinition = EmployeeDefinition

type AgentMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PromptFragments struct {
	Identity            PromptFragment `json:"identity,omitempty"`
	Company             PromptFragment `json:"company,omitempty"`
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
	botToken, err := loadSlackBotToken(ctx, deps, *agent.OrgID)
	if err != nil {
		return nil, err
	}
	appToken := ""
	if deps.Cfg != nil {
		appToken = strings.TrimSpace(deps.Cfg.SlackAppToken)
	}
	if appToken == "" {
		return nil, fmt.Errorf("employee runtime startup: SLACK_APP_TOKEN is required")
	}
	proxyToken, err := MintProxyToken(ctx, deps, agent, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return &StartupSecrets{
		SlackBotToken: botToken,
		SlackAppToken: appToken,
		ProxyToken:    proxyToken.Token,
	}, nil
}

func MintProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID) (*ProxyTokenResult, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("employee runtime proxy token: agent must have org_id")
	}
	if deps.DB == nil {
		return nil, fmt.Errorf("employee runtime proxy token: db is required")
	}
	if len(deps.SigningKey) == 0 {
		return nil, fmt.Errorf("employee runtime proxy token: signing key is required")
	}
	cred, err := credentials.Resolve(ctx, deps.DB, deps.Picker, agent)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}
	rawToken, jti, err := token.Mint(
		deps.SigningKey,
		agent.OrgID.String(),
		cred.ID.String(),
		proxyTokenTTL,
		token.MintOptions{IsSystem: cred.IsSystem},
	)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	now := time.Now().UTC()
	meta := model.JSON{"employee_id": agent.ID.String(), "type": "employee_proxy", "harness": "employee-sandbox"}
	if sandboxID != uuid.Nil {
		meta["sandbox_id"] = sandboxID.String()
	}
	expiresAt := now.Add(proxyTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       model.JSON{},
		Meta:         meta,
	}
	if err := deps.DB.WithContext(ctx).Create(&dbToken).Error; err != nil {
		return nil, fmt.Errorf("persist proxy token: %w", err)
	}
	return &ProxyTokenResult{
		Token:     "ptok_" + rawToken,
		JTI:       jti,
		ExpiresAt: expiresAt,
	}, nil
}

func AttachLatestProxyTokenToSandbox(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID) error {
	if agent == nil || agent.OrgID == nil || sandboxID == uuid.Nil || deps.DB == nil {
		return nil
	}
	var tok model.Token
	if err := deps.DB.WithContext(ctx).
		Where("org_id = ? AND meta->>'employee_id' = ? AND meta->>'type' = ? AND meta->>'harness' = ?",
			*agent.OrgID, agent.ID.String(), "employee_proxy", "employee-sandbox").
		Order("created_at DESC").
		First(&tok).Error; err != nil {
		return nil
	}
	meta := tok.Meta
	if meta == nil {
		meta = model.JSON{}
	}
	meta["sandbox_id"] = sandboxID.String()
	return deps.DB.WithContext(ctx).Model(&tok).Update("meta", meta).Error
}

func Compile(ctx context.Context, deps CompileDeps, agent *model.Agent) (*EmployeeDefinition, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("employee runtime compile: agent must have org_id")
	}
	skills, err := buildSkills(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, err
	}
	mcpServers := jsonArray(agent.McpServers)
	if ourMCP := buildEmployeeMCPServer(ctx, deps, agent); ourMCP != nil {
		mcpServers = upsertHivyMCPServer(mcpServers, ourMCP)
	}
	description := managedEmployeeDescription
	fragments := buildPromptFragments(ctx, deps.DB, agent, description)
	contextMap := map[string]any{
		"memory": buildMemoryContext(ctx, deps, agent),
	}
	slackConfig := buildSlackConfig(ctx, deps, agent)
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultEmployeeModel
	}
	return &EmployeeDefinition{
		Employee: AgentMeta{
			Name:        managedEmployeeName,
			Description: description,
		},
		PromptFragments:  fragments,
		Model:            proxyModel(deps.Cfg, modelID),
		MultimodalModel:  ptrModel(proxyModel(deps.Cfg, DefaultEmployeeMultimodalModel)),
		Limits:           defaultLimits(),
		Context:          contextMap,
		Tools:            defaultTools(),
		McpServers:       mcpServers,
		Skills:           skills,
		Specialists:      []any{},
		Slack:            slackConfigMap(slackConfig),
		OutboundChannels: []any{},
	}, nil
}

func ControlPlaneOutboundChannels(cfg *config.Config, sandboxID uuid.UUID) []any {
	bridgeHost := "api.usehivy.com"
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
