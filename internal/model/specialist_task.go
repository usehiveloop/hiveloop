package model

import (
	"time"

	"github.com/google/uuid"
)

type SpecialistTask struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_specialist_task_org"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`

	SpecialistID uuid.UUID `gorm:"type:uuid;not null;index"`
	Specialist   Employee  `gorm:"foreignKey:SpecialistID;constraint:OnDelete:CASCADE"`

	SandboxID      uuid.UUID            `gorm:"type:uuid;not null"`
	Sandbox        Sandbox              `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	ConversationID uuid.UUID            `gorm:"type:uuid;not null"`
	Conversation   EmployeeConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`

	ParentConversationType string `gorm:"not null"`
	ParentConversationID   string `gorm:"not null;index"`

	Brief    string `gorm:"type:text;not null"`
	Metadata JSON   `gorm:"type:jsonb;default:'{}'"`

	CreatedAt time.Time
}

func (SpecialistTask) TableName() string { return "specialist_tasks" }
