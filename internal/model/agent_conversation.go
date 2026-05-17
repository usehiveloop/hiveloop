package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentConversation struct {
	ID                    uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                 uuid.UUID   `gorm:"type:uuid;not null;index:idx_conv_org_agent"`
	Org                   Org         `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID               uuid.UUID   `gorm:"type:uuid;not null;index:idx_conv_org_agent"`
	Agent                 Agent       `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SandboxID             uuid.UUID   `gorm:"type:uuid;not null"`
	Sandbox               Sandbox     `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	RuntimeConversationID string      `gorm:"not null;index"`
	Source                string      `gorm:"not null;default:'';index"`
	SourceID              *uuid.UUID  `gorm:"type:uuid;index"`
	SourceResourceKey     string      `gorm:"not null;default:'';index"`
	Status                string      `gorm:"not null;default:'active'"` // active, ended, error
	Name                  string      `gorm:"type:text"`                 // auto-generated title, set asynchronously after first message
	CreatedAt             time.Time
	UpdatedAt             time.Time
	EndedAt               *time.Time
}

func (AgentConversation) TableName() string { return "agent_conversations" }
