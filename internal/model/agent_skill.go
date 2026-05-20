package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentSkill attaches a Skill to an Employee. PinnedVersionID is nil when the
// employee should follow the skill's latest version; otherwise the employee is
// frozen to that specific version.
type AgentSkill struct {
	AgentID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SkillID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Skill   Skill     `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`

	PinnedVersionID *uuid.UUID    `gorm:"type:uuid"`
	PinnedVersion   *SkillVersion `gorm:"foreignKey:PinnedVersionID;constraint:OnDelete:SET NULL"`

	CreatedAt time.Time
}

func (AgentSkill) TableName() string { return "employee_skills" }
