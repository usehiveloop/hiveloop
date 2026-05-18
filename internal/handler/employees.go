package handler

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const (
	employeeHarness             = "employee-sandbox"
	employeeCloudAgentHarness   = "open_code"
	employeeCategoryEngineering = "engineering"
	engineeringTeamName         = "Engineering"
)

var defaultEmployeeSkills = map[string][]string{
	employeeCategoryEngineering: {"git-github", "asset-uploads"},
}

type EmployeeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	agents       *AgentHandler
	enqueuer     enqueue.TaskEnqueuer
	taskCleaner  enqueue.TaskCleaner
}

func NewEmployeeHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps, agents *AgentHandler) *EmployeeHandler {
	return &EmployeeHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps, agents: agents}
}

func (h *EmployeeHandler) SetEnqueuer(enq enqueue.TaskEnqueuer) {
	h.enqueuer = enq
	if cleaner, ok := enq.(enqueue.TaskCleaner); ok {
		h.taskCleaner = cleaner
	}
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
		{"openrouter", employeeruntime.DefaultEmployeeModel},
	})
}

func pickEmployeeSubagentCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredential(db, []struct{ providerID, modelID string }{
		{"openrouter", employeeruntime.DefaultEmployeeSubagentModel},
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
