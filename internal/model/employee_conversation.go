package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeConversation struct {
	ID                    uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                 uuid.UUID   `gorm:"type:uuid;not null;index:idx_employee_session_org_employee"`
	Org                   Org         `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	EmployeeID            uuid.UUID   `gorm:"type:uuid;not null;index:idx_employee_session_org_employee"`
	Employee              Employee    `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	SandboxID             uuid.UUID   `gorm:"type:uuid;not null"`
	Sandbox               Sandbox     `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	RuntimeConversationID string      `gorm:"not null;index"`
	Source                string      `gorm:"not null;default:'';index"`
	SourceID              *uuid.UUID  `gorm:"type:uuid;index"`
	SourceResourceKey     string      `gorm:"not null;default:'';index"`
	CredentialID          *uuid.UUID  `gorm:"type:uuid;index"`
	Credential            *Credential `gorm:"foreignKey:CredentialID;constraint:OnDelete:SET NULL"`
	TokenID               *uuid.UUID  `gorm:"type:uuid"`
	Token                 *Token      `gorm:"foreignKey:TokenID;constraint:OnDelete:SET NULL"`
	Status                string      `gorm:"not null;default:'active'"` // active, ended, error
	Name                  string      `gorm:"type:text"`                 // auto-generated title, set asynchronously after first message
	IntegrationScopes     JSON        `gorm:"type:jsonb;default:'{}'"`   // which integrations this conversation has
	CreatedAt             time.Time
	UpdatedAt             time.Time
	EndedAt               *time.Time
}

func (EmployeeConversation) TableName() string { return "employee_sessions" }
