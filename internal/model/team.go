package model

import (
	"time"

	"github.com/google/uuid"
)

type Team struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org         Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name        string     `gorm:"not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	PromptTeam  string     `gorm:"type:text;not null;default:''"`
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Team) TableName() string { return "teams" }
