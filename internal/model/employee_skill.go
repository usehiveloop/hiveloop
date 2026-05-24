package model

import (
	"time"

	"github.com/google/uuid"
)

// EmployeeSkill attaches a Skill to an Employee.
type EmployeeSkill struct {
	EmployeeID uuid.UUID `gorm:"type:uuid;primaryKey"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`
	SkillID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Skill      Skill     `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time
}

func (EmployeeSkill) TableName() string { return "employee_skills" }
