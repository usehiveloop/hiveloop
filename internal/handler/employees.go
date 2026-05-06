package handler

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/hermes"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const (
	employeeHarness             = "hermes"
	employeeCategoryEngineering = "engineering"
	engineeringTeamName         = "Engineering"
)

var defaultEmployeeSkills = map[string][]string{
	employeeCategoryEngineering: {"git-github", "employee-public-assets-uploads"},
}

var defaultEmployeeSubagentSkills = map[string][]string{
	employeeCategoryEngineering: {"agent-browser", "git-github", "public-assets-uploads"},
}

type EmployeeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  hermes.CompileDeps
	agents       *AgentHandler
}

func NewEmployeeHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps hermes.CompileDeps, agents *AgentHandler) *EmployeeHandler {
	return &EmployeeHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps, agents: agents}
}

type createEmployeeRequest struct {
	Category    string `json:"category"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url"`
	Description string `json:"description"`
}

type createEmployeeResponse struct {
	AgentID   string `json:"agent_id"`
	SandboxID string `json:"sandbox_id"`
	Status    string `json:"status"`
}

type employeeProviderChoice struct {
	cred  *model.Credential
	model string
}

func pickEmployeeCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredential(db, []struct{ providerID, modelID string }{
		{"crof", "deepseek-v4-pro-precision"},
		{"openrouter", "deepseek/deepseek-v4-pro"},
	})
}

func pickEmployeeSubagentCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredential(db, []struct{ providerID, modelID string }{
		{"openrouter", "moonshotai/kimi-k2.6"},
		{"crof", "deepseek-v4-pro-precision"},
	})
}

func pickSystemCredential(db *gorm.DB, candidates []struct{ providerID, modelID string }) (*employeeProviderChoice, error) {
	for _, c := range candidates {
		var cred model.Credential
		err := db.Where("is_system = ? AND provider_id = ? AND revoked_at IS NULL", true, c.providerID).
			Order("created_at ASC").
			Limit(1).
			First(&cred).Error
		if err == nil {
			return &employeeProviderChoice{cred: &cred, model: c.modelID}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("lookup %s system credential: %w", c.providerID, err)
		}
	}
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.providerID
	}
	return nil, fmt.Errorf("no %v system credential configured", names)
}
