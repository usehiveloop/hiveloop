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
	"github.com/usehivy/hivy/internal/specialists"
	"github.com/usehivy/hivy/internal/token"
)

const (
	DefaultEmployeeModel           = "deepseek-v4-flash"
	DefaultEmployeeSpecialistModel = "deepseek-v4-pro"
	DefaultEmployeeMultimodalModel = "gemini-3-flash-preview"
	proxyTokenTTL                  = 24 * time.Hour
	managedEmployeeName            = "Hivy"
	managedEmployeeDescription     = "Hivy is the organization's managed AI employee."
)

type CompileDeps struct {
	DB          *gorm.DB
	Picker      credentials.Picker
	KMS         *crypto.KeyWrapper
	EncKey      *crypto.SymmetricKey
	SigningKey  []byte
	Cfg         *config.Config
	Nango       *nango.Client
	Hindsight   HindsightRecallClient
	Specialists *specialists.Catalog
}

type HindsightRecallClient interface {
	Recall(ctx context.Context, bankID string, req *hindsight.RecallRequest) (*hindsight.RecallResponse, error)
}

type StartupSecrets struct {
	ProxyToken string
}

type ProxyTokenResult struct {
	Token     string
	JTI       string
	ExpiresAt time.Time
}

type EmployeeDefinition struct {
	Agent             AgentMeta          `json:"agent"`
	Mode              string             `json:"mode,omitempty"`
	SpecialistProfile *SpecialistProfile `json:"specialist_profile,omitempty"`
	SystemPrompt      SystemPromptConfig `json:"system_prompt"`
	Model             ModelConfig        `json:"model"`
	MultimodalModel   *ModelConfig       `json:"multimodal_model,omitempty"`
	Limits            map[string]any     `json:"limits,omitempty"`
	Context           map[string]any     `json:"context,omitempty"`
	Tools             []map[string]any   `json:"tools"`
	McpServers        []any              `json:"mcp_servers"`
	Skills            []SkillSpec        `json:"skills"`
	OutboundChannels  []any              `json:"outbound_channels"`
}

type AgentDefinition = EmployeeDefinition

type AgentMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SpecialistProfile struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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

func PrepareStartup(ctx context.Context, deps CompileDeps, agent *model.Employee) (*StartupSecrets, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("employee runtime startup: agent must have org_id")
	}
	proxyToken, err := MintProxyToken(ctx, deps, agent, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return &StartupSecrets{
		ProxyToken: proxyToken.Token,
	}, nil
}

func MintProxyToken(ctx context.Context, deps CompileDeps, agent *model.Employee, sandboxID uuid.UUID) (*ProxyTokenResult, error) {
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
		token.MintOptions{IsSystem: cred.OrgID == nil},
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

func AttachLatestProxyTokenToSandbox(ctx context.Context, deps CompileDeps, agent *model.Employee, sandboxID uuid.UUID) error {
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

func Compile(ctx context.Context, deps CompileDeps, agent *model.Employee) (*EmployeeDefinition, error) {
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
	fragments := buildPromptSections(ctx, deps.DB, agent, description)
	fragments.AvailableSpecialists = buildAvailableSpecialistsSection(agent, deps.Specialists)
	contextMap := map[string]any{
		"memory": buildMemoryContext(ctx, deps, agent),
	}
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultEmployeeModel
	}
	return &EmployeeDefinition{
		Agent: AgentMeta{
			Name:        managedEmployeeName,
			Description: description,
		},
		Mode:              "employee",
		SpecialistProfile: nil,
		SystemPrompt:      buildEmployeeSystemPrompt(fragments),
		Model:             proxyModel(deps.Cfg, modelID),
		MultimodalModel:   ptrModel(proxyModel(deps.Cfg, DefaultEmployeeMultimodalModel)),
		Limits:            defaultLimits(),
		Context:           contextMap,
		Tools:             defaultTools(),
		McpServers:        mcpServers,
		Skills:            skills,
		OutboundChannels:  []any{},
	}, nil
}

func CompileSpecialist(ctx context.Context, deps CompileDeps, employee *model.Employee, def specialists.Definition) (*EmployeeDefinition, error) {
	if employee == nil || employee.OrgID == nil {
		return nil, fmt.Errorf("specialist runtime compile: employee must have org_id")
	}
	skills, err := buildSkillsWithDefaultNames(ctx, deps.DB, employee.ID, def.DefaultSkillNames)
	if err != nil {
		return nil, err
	}
	mcpServers := jsonArray(employee.McpServers)
	if ourMCP := buildEmployeeMCPServer(ctx, deps, employee); ourMCP != nil {
		mcpServers = upsertHivyMCPServer(mcpServers, ourMCP)
	}
	fragments := buildPromptSections(ctx, deps.DB, employee, def.Description)
	modelID := strings.TrimSpace(employee.Model)
	if modelID == "" {
		modelID = DefaultEmployeeSpecialistModel
	}
	return &EmployeeDefinition{
		Agent: AgentMeta{
			Name:        def.Name,
			Description: def.Description,
		},
		Mode: "specialist",
		SpecialistProfile: &SpecialistProfile{
			Name:        def.Name,
			Description: def.Description,
		},
		SystemPrompt:     buildSpecialistSystemPrompt(fragments, def),
		Model:            proxyModel(deps.Cfg, modelID),
		MultimodalModel:  ptrModel(proxyModel(deps.Cfg, DefaultEmployeeMultimodalModel)),
		Limits:           defaultLimits(),
		Context:          map[string]any{"memory": buildMemoryContext(ctx, deps, employee)},
		Tools:            defaultTools(),
		McpServers:       mcpServers,
		Skills:           skills,
		OutboundChannels: []any{},
	}, nil
}

func ControlPlaneOutboundChannels(cfg *config.Config, sandboxID uuid.UUID) []any {
	baseURL := cfg.RuntimeControlPlaneBaseURL()
	return []any{
		map[string]any{
			"name":       "control-plane-memory",
			"type":       "webhook",
			"url":        fmt.Sprintf("%s/internal/webhooks/employee/%s", baseURL, sandboxID),
			"secret_env": EmployeeEnvRuntimeSecret,
		},
	}
}
