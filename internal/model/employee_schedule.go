package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeSchedule struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Org        Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_employee_schedule_employee_bridge"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	SandboxID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Sandbox    Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`

	BridgeJobID     string `gorm:"not null;size:255;uniqueIndex:idx_employee_schedule_employee_bridge"`
	Status          string `gorm:"not null;default:'active';size:64;index"`
	Channel         string `gorm:"not null;default:'';size:255"`
	Description     string `gorm:"type:text;not null;default:''"`
	TaskPrompt      string `gorm:"type:text;not null;default:''"`
	IntervalSeconds *int64
	RepeatCount     *int64
	RepeatCompleted int64 `gorm:"not null;default:0"`

	NextRunAt        *time.Time `gorm:"index"`
	LastRunAt        *time.Time
	LastStatus       string `gorm:"not null;default:'';size:64"`
	LastError        string `gorm:"type:text;not null;default:''"`
	CreatedBySession string `gorm:"not null;default:'';size:255"`
	BridgeCreatedAt  *time.Time
	CancelledAt      *time.Time `gorm:"index"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (EmployeeSchedule) TableName() string { return "employee_schedules" }

type EmployeeScheduleRun struct {
	ID         uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID        `gorm:"type:uuid;not null;index"`
	Org        Org              `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID uuid.UUID        `gorm:"type:uuid;not null;index"`
	Employee   Employee         `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	ScheduleID uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_employee_schedule_run_key"`
	Schedule   EmployeeSchedule `gorm:"foreignKey:ScheduleID;constraint:OnDelete:CASCADE"`
	SandboxID  uuid.UUID        `gorm:"type:uuid;not null;index"`
	Sandbox    Sandbox          `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`

	BridgeJobID  string     `gorm:"not null;size:255;index"`
	RunKey       string     `gorm:"not null;size:500;uniqueIndex:idx_employee_schedule_run_key"`
	Status       string     `gorm:"not null;default:'running';size:64;index"`
	ScheduledAt  *time.Time `gorm:"index"`
	StartedAt    *time.Time
	CompletedAt  *time.Time
	DurationMS   *int64
	Error        string  `gorm:"type:text;not null;default:''"`
	EventPayload RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (EmployeeScheduleRun) TableName() string { return "employee_schedule_runs" }
