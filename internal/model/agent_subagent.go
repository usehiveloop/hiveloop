package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentSubagent links a parent Agent to a child Agent of type "subagent".
// A subagent can be attached to many parent agents (many-to-many).
type AgentSubagent struct {
	AgentID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Agent      Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SubagentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Subagent   Agent     `gorm:"foreignKey:SubagentID;constraint:OnDelete:CASCADE"`
	CreatedAt  time.Time
}

func (AgentSubagent) TableName() string { return "agent_subagents" }
