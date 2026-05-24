package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeMemoryEvent struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID `gorm:"type:uuid;not null;index:idx_employee_memory_scope"`
	Org        Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index:idx_employee_memory_scope"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	SandboxID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Sandbox    Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`

	SessionID        string     `gorm:"not null;index:idx_employee_memory_scope;size:255"`
	EventType        string     `gorm:"not null;index;size:128"`
	Source           string     `gorm:"not null;default:'manual';size:128"`
	Mode             string     `gorm:"not null;default:'employee';index;size:64"`
	SpecialistSlug   string     `gorm:"not null;default:'';index;size:128"`
	SpecialistTaskID *uuid.UUID `gorm:"type:uuid;index"`
	Payload          RawJSON    `gorm:"type:jsonb;not null;default:'{}'"`
	EventAt          time.Time  `gorm:"not null;index"`

	RetainedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time
}

func (EmployeeMemoryEvent) TableName() string { return "employee_memory_events" }
