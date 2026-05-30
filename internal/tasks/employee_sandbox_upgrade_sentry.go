package tasks

import (
	"context"
	"fmt"

	sentrygo "github.com/getsentry/sentry-go"

	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

func annotateEmployeeSandboxUpgradeSentry(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, agent *model.Employee, oldSandbox *model.Sandbox) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("feature", "employee_sandbox_upgrade")
	hub.Scope().SetTag("employee.upgrade_id", upgrade.ID.String())
	hub.Scope().SetTag("employee.employee_id", upgrade.EmployeeID.String())
	hub.Scope().SetTag("employee.org_id", upgrade.OrgID.String())
	if oldSandbox != nil {
		hub.Scope().SetTag("employee.old_sandbox_id", oldSandbox.ID.String())
	}
	setEmployeeUpgradeContext(hub, upgrade, oldSandbox, nil)
	addEmployeeUpgradeBreadcrumb(ctx, "started", sentrygo.LevelInfo, sentrygo.Context{
		"status": upgrade.Status,
		"phase":  upgrade.Phase,
	})
}

func recordEmployeeSandboxUpgradePhase(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase string) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("employee.upgrade_phase", phase)
	setEmployeeUpgradeContext(hub, upgrade, nil, nil)
	addEmployeeUpgradeBreadcrumb(ctx, "phase "+phase, sentrygo.LevelInfo, sentrygo.Context{
		"phase": phase,
	})
}

func recordEmployeeSandboxUpgradeNewSandbox(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, sb *model.Sandbox) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil || sb == nil {
		return
	}
	hub.Scope().SetTag("employee.new_sandbox_id", sb.ID.String())
	setEmployeeUpgradeContext(hub, upgrade, nil, sentrygo.Context{
		"new_sandbox_id":          sb.ID.String(),
		"new_sandbox_external_id": sb.ExternalID,
	})
}

func recordEmployeeSandboxUpgradeFailure(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase, message string) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("employee.upgrade_phase", phase)
	hub.Scope().SetTag("employee.upgrade_status", model.EmployeeSandboxUpgradeStatusFailed)
	setEmployeeUpgradeContext(hub, upgrade, nil, sentrygo.Context{
		"failed_phase": phase,
		"error":        message,
	})
	addEmployeeUpgradeBreadcrumb(ctx, "failed at "+phase, sentrygo.LevelError, sentrygo.Context{
		"phase": phase,
		"error": message,
	})
}

func recordEmployeeSandboxUpgradeSuccess(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil {
		return
	}
	hub.Scope().SetTag("employee.upgrade_phase", model.EmployeeSandboxUpgradePhaseCompleted)
	hub.Scope().SetTag("employee.upgrade_status", model.EmployeeSandboxUpgradeStatusSucceeded)
	setEmployeeUpgradeContext(hub, upgrade, nil, nil)
	addEmployeeUpgradeBreadcrumb(ctx, "completed", sentrygo.LevelInfo, sentrygo.Context{
		"status": model.EmployeeSandboxUpgradeStatusSucceeded,
	})
}

func recordEmployeeSandboxRetire(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, sb *model.Sandbox) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil || upgrade == nil || sb == nil {
		return
	}
	hub.Scope().SetTag("feature", "employee_sandbox_upgrade")
	hub.Scope().SetTag("employee.upgrade_id", upgrade.ID.String())
	hub.Scope().SetTag("employee.employee_id", upgrade.EmployeeID.String())
	hub.Scope().SetTag("employee.old_sandbox_id", sb.ID.String())
	setEmployeeUpgradeContext(hub, upgrade, sb, nil)
	addEmployeeUpgradeBreadcrumb(ctx, "retire old sandbox", sentrygo.LevelInfo, sentrygo.Context{
		"sandbox_id":  sb.ID.String(),
		"external_id": sb.ExternalID,
	})
}

func employeeUpgradeHub(ctx context.Context) *sentrygo.Hub {
	if !sentryobs.Enabled() {
		return nil
	}
	if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
		return hub
	}
	return sentrygo.CurrentHub()
}

func setEmployeeUpgradeContext(hub *sentrygo.Hub, upgrade *model.EmployeeSandboxUpgrade, oldSandbox *model.Sandbox, extra sentrygo.Context) {
	data := sentrygo.Context{
		"upgrade_id":  upgrade.ID.String(),
		"org_id":      upgrade.OrgID.String(),
		"employee_id": upgrade.EmployeeID.String(),
		"status":      upgrade.Status,
		"phase":       upgrade.Phase,
	}
	if upgrade.OldSandboxID != nil {
		data["old_sandbox_id"] = upgrade.OldSandboxID.String()
	}
	if upgrade.NewSandboxID != nil {
		data["new_sandbox_id"] = upgrade.NewSandboxID.String()
	}
	if oldSandbox != nil {
		data["old_sandbox_external_id"] = oldSandbox.ExternalID
		data["old_sandbox_status"] = oldSandbox.Status
	}
	for key, value := range extra {
		data[key] = value
	}
	hub.Scope().SetContext("employee_sandbox_upgrade", data)
}

func addEmployeeUpgradeBreadcrumb(ctx context.Context, message string, level sentrygo.Level, data sentrygo.Context) {
	hub := employeeUpgradeHub(ctx)
	if hub == nil {
		return
	}
	hub.AddBreadcrumb(&sentrygo.Breadcrumb{
		Type:     "default",
		Category: "employee_sandbox_upgrade",
		Message:  fmt.Sprintf("employee sandbox upgrade %s", message),
		Level:    level,
		Data:     data,
	}, nil)
}
