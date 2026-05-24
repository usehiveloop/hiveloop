package handler

import (
	"context"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	employeeHarness           = "employee-sandbox"
	employeeCloudAgentHarness = "open_code"
	hivyEmployeeName          = "Hivy"
	hivyEmployeeDescription   = "Hivy is the organization's managed AI employee."
	hivyEmployeeAvatarURL     = "/assets/hivy-avatar.png"
)

var defaultEmployeeSkills = []string{"asset-uploads"}

type EmployeeHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	registry     *registry.Registry
	enqueuer     enqueue.TaskEnqueuer
	taskCleaner  enqueue.TaskCleaner
}

func NewEmployeeHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps, reg *registry.Registry) *EmployeeHandler {
	return &EmployeeHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps, registry: reg}
}

func (h *EmployeeHandler) SetEnqueuer(enq enqueue.TaskEnqueuer) {
	h.enqueuer = enq
	if cleaner, ok := enq.(enqueue.TaskCleaner); ok {
		h.taskCleaner = cleaner
	}
}

type employeeProviderChoice struct {
	cred  *model.Credential
	model string
}

func pickEmployeeCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredentialByModel(db, employeeruntime.DefaultEmployeeModel)
}

func pickEmployeeSubagentCredential(db *gorm.DB) (*employeeProviderChoice, error) {
	return pickSystemCredentialByModel(db, employeeruntime.DefaultEmployeeSubagentModel)
}

func pickSystemCredentialByModel(db *gorm.DB, modelID string) (*employeeProviderChoice, error) {
	cred, err := pickActiveSystemCredentialForModel(context.Background(), db, registry.Global(), modelID)
	if err != nil {
		return nil, err
	}
	return &employeeProviderChoice{cred: cred, model: modelID}, nil
}
