package evals

import (
	"context"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

func syncTrialEmployee(ctx context.Context, deps *bootstrap.Deps, employee *model.Employee) (*model.Sandbox, error) {
	employeeHandler := handler.NewEmployeeHandler(deps.DB, deps.Orchestrator, compileDeps(deps), deps.Registry, deps.Specialists)
	sb, _, err := employeeHandler.SyncEmployee(ctx, employee)
	return sb, err
}

func compileDeps(deps *bootstrap.Deps) employeeruntime.CompileDeps {
	return employeeruntime.CompileDeps{
		DB:          deps.DB,
		Picker:      credentials.NewPickerWithRegistry(deps.DB, deps.Registry),
		KMS:         deps.KMS,
		EncKey:      deps.SandboxEncKey,
		SigningKey:  deps.SigningKey,
		Cfg:         deps.Config,
		Nango:       deps.NangoClient,
		Hindsight:   deps.HindsightClient,
		Specialists: deps.Specialists,
	}
}
