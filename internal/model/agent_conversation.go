package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentConversation struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID  `gorm:"type:uuid;not null;index:idx_conv_org_agent"`
	Org                  Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID              uuid.UUID  `gorm:"type:uuid;not null;index:idx_conv_org_agent"`
	Agent                Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SandboxID            uuid.UUID  `gorm:"type:uuid;not null"`
	Sandbox              Sandbox    `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	BridgeConversationID string     `gorm:"not null;index"`
	CredentialID         *uuid.UUID `gorm:"type:uuid;index"` // user's credential for system agent conversations
	Credential           *Credential `gorm:"foreignKey:CredentialID;constraint:OnDelete:SET NULL"`
	TokenID              *uuid.UUID `gorm:"type:uuid"`
	Token                *Token     `gorm:"foreignKey:TokenID;constraint:OnDelete:SET NULL"`
	Status               string     `gorm:"not null;default:'active'"` // active, ended, error
	IntegrationScopes    JSON       `gorm:"type:jsonb;default:'{}'"` // which integrations this conversation has
	CreatedAt            time.Time
	UpdatedAt            time.Time
	EndedAt              *time.Time
}

func (AgentConversation) TableName() string { return "agent_conversations" }
