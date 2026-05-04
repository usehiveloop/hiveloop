package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentSubagent links an employee Agent (is_employee=true) to a child Agent it owns.
// An agent can be a subagent of many employees (many-to-many).
type AgentSubagent struct {
	AgentID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Agent      Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SubagentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Subagent   Agent     `gorm:"foreignKey:SubagentID;constraint:OnDelete:CASCADE"`
	CreatedAt  time.Time
}

func (AgentSubagent) TableName() string { return "agent_subagents" }
