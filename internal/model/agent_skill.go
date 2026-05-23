package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentSkill attaches a Skill to an Employee.
type AgentSkill struct {
	AgentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SkillID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Skill   Skill     `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time
}

func (AgentSkill) TableName() string { return "employee_skills" }
