package evals

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func syncTrialEmployee(ctx context.Context, deps *bootstrap.Deps, employee *model.Employee) (*model.Sandbox, error) {
	compileDeps := compileDeps(deps)
	secrets, err := employeeruntime.PrepareStartup(ctx, compileDeps, employee)
	if err != nil {
		return nil, fmt.Errorf("prepare employee startup: %w", err)
	}
	sb, err := deps.Orchestrator.CreateEmployeeSandbox(ctx, employee, secrets)
	if err != nil {
		return nil, fmt.Errorf("create employee sandbox: %w", err)
	}
	if err := employeeruntime.AttachLatestProxyTokenToSandbox(ctx, compileDeps, employee, sb.ID); err != nil {
		return nil, fmt.Errorf("attach proxy token to sandbox: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, compileDeps, employee)
	if err != nil {
		return nil, fmt.Errorf("compile employee config: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(deps.Config, sb.ID)
	apiKey, err := deps.SandboxEncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt sandbox api key: %w", err)
	}
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutConfig(ctx, def); err != nil {
		return nil, fmt.Errorf("push employee config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime readyz: %w", err)
	}
	if employee.Status != "active" {
		if err := deps.DB.WithContext(ctx).Model(employee).Update("status", "active").Error; err != nil {
			return nil, fmt.Errorf("mark employee active: %w", err)
		}
		employee.Status = "active"
	}
	return sb, nil
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
