package model

import (
	"time"

	"github.com/google/uuid"
)

type CloudAgentTask struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_cloud_task_org"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	EmployeeAgentID uuid.UUID `gorm:"type:uuid;not null;index"`
	EmployeeAgent   Agent     `gorm:"foreignKey:EmployeeAgentID;constraint:OnDelete:CASCADE"`

	CloudAgentID uuid.UUID `gorm:"type:uuid;not null;index"`
	CloudAgent   Agent     `gorm:"foreignKey:CloudAgentID;constraint:OnDelete:CASCADE"`

	SandboxID      uuid.UUID         `gorm:"type:uuid;not null"`
	Sandbox        Sandbox           `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	ConversationID uuid.UUID         `gorm:"type:uuid;not null"`
	Conversation   AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`

	ParentConversationType string `gorm:"not null"`
	ParentConversationID   string `gorm:"not null;index"`

	Brief    string `gorm:"type:text;not null"`
	Metadata JSON   `gorm:"type:jsonb;default:'{}'"`

	CreatedAt time.Time
}

func (CloudAgentTask) TableName() string { return "specialist_tasks" }
