package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeAsset struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index"`
	AgentID   uuid.UUID `gorm:"type:uuid;not null;index:idx_emp_asset_agent_created,priority:1"`
	SandboxID uuid.UUID `gorm:"type:uuid;not null"`

	Path        string `gorm:"type:text;not null"`
	Filename    string `gorm:"type:text;not null"`
	Key         string `gorm:"type:text;not null;uniqueIndex"`
	PublicURL   string `gorm:"type:text;not null"`
	ContentType string `gorm:"type:text;not null"`
	Bytes       int64  `gorm:"not null"`

	CreatedAt time.Time `gorm:"index:idx_emp_asset_agent_created,priority:2,sort:desc"`
	UpdatedAt time.Time
}

func (EmployeeAsset) TableName() string { return "employee_assets" }
