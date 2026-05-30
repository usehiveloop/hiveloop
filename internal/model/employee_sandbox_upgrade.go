package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	EmployeeSandboxUpgradeStatusQueued    = "queued"
	EmployeeSandboxUpgradeStatusRunning   = "running"
	EmployeeSandboxUpgradeStatusSucceeded = "succeeded"
	EmployeeSandboxUpgradeStatusFailed    = "failed"

	EmployeeSandboxUpgradePhaseQueued      = "queued"
	EmployeeSandboxUpgradePhaseCreatingNew = "creating_new"
	EmployeeSandboxUpgradePhaseSync        = "sync"
	EmployeeSandboxUpgradePhasePausingOld  = "pausing_old"
	EmployeeSandboxUpgradePhaseCleanupOld  = "cleanup_old"
	EmployeeSandboxUpgradePhaseCompleted   = "completed"
	EmployeeSandboxUpgradePhaseFailed      = "failed"
)

type EmployeeSandboxUpgrade struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`

	OldSandboxID *uuid.UUID `gorm:"type:uuid;index"`
	OldSandbox   *Sandbox   `gorm:"foreignKey:OldSandboxID;constraint:OnDelete:SET NULL"`
	NewSandboxID *uuid.UUID `gorm:"type:uuid;index"`
	NewSandbox   *Sandbox   `gorm:"foreignKey:NewSandboxID;constraint:OnDelete:SET NULL"`

	Status string `gorm:"not null;default:'queued';size:32;index"`
	Phase  string `gorm:"not null;default:'queued';size:64"`

	ErrorMessage *string
	CompletedAt  *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (EmployeeSandboxUpgrade) TableName() string { return "employee_sandbox_upgrades" }
