package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// EmployeeTrigger links an employee to an inbound trigger source.
type EmployeeTrigger struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org          Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	Employee     Employee       `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	TriggerType  string         `gorm:"not null;default:'webhook';size:32;index"` // webhook or http
	ConnectionID *uuid.UUID     `gorm:"type:uuid;index"`
	Connection   *Connection  `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	TriggerKeys  pq.StringArray `gorm:"type:text[];not null;default:'{}'"` // e.g. {"issues.opened","issues.reopened"}
	Enabled      bool           `gorm:"not null;default:true"`
	Conditions   RawJSON        `gorm:"type:jsonb"`                    // TriggerMatch JSON
	Instructions string         `gorm:"type:text;not null;default:''"` // per-trigger task instruction
	SecretKey    string         `gorm:"type:text;not null;default:''"` // bcrypt hash for HTTP triggers

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (EmployeeTrigger) TableName() string { return "employee_triggers" }

// TriggerMatch defines filtering conditions on the webhook payload.
type TriggerMatch struct {
	Mode       string             `json:"mode"` // "all" (AND) or "any" (OR)
	Conditions []TriggerCondition `json:"conditions"`
}

// TriggerCondition is a single filter rule applied to the webhook payload.
type TriggerCondition struct {
	Path     string `json:"path"`     // dot-path into payload, e.g. "repository.full_name"
	Operator string `json:"operator"` // equals, not_equals, one_of, not_one_of, contains, not_contains, matches, exists, not_exists
	Value    any    `json:"value"`    // string or []string depending on operator
}
