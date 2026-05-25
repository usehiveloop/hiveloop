package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeSessionEvent struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID       `gorm:"type:uuid;not null;index:idx_employee_session_event_scope"`
	Org               Org             `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID        uuid.UUID       `gorm:"type:uuid;not null;index:idx_employee_session_event_scope"`
	Employee          Employee        `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	SandboxID         uuid.UUID       `gorm:"type:uuid;not null;index"`
	Sandbox           Sandbox         `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	EmployeeSessionID uuid.UUID       `gorm:"type:uuid;not null;index"`
	EmployeeSession   EmployeeSession `gorm:"foreignKey:EmployeeSessionID;constraint:OnDelete:CASCADE"`

	SessionID        string     `gorm:"column:runtime_session_id;not null;index:idx_employee_session_event_scope;size:255"`
	EventID          string     `gorm:"not null;default:'';index;size:255"`
	EventType        string     `gorm:"not null;index;size:128"`
	Source           string     `gorm:"not null;default:'manual';size:128"`
	Mode             string     `gorm:"not null;default:'employee';index;size:64"`
	SpecialistSlug   string     `gorm:"not null;default:'';index;size:128"`
	SpecialistTaskID *uuid.UUID `gorm:"type:uuid;index"`
	SequenceNumber   int64      `gorm:"not null;default:0;index"`
	Payload          RawJSON    `gorm:"type:jsonb;not null;default:'{}'"`
	EventAt          time.Time  `gorm:"not null;index"`

	RetainedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time
}

func (EmployeeSessionEvent) TableName() string { return "employee_session_events" }
