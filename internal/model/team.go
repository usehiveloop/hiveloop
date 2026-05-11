package model

import (
	"time"

	"github.com/google/uuid"
)

type Team struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	// Uniqueness is enforced by a partial index created in AutoMigrate
	// (idx_team_org_name) so soft-deleted rows don't block name reuse.
	Name        string     `gorm:"not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	PromptTeam  string     `gorm:"type:text;not null;default:''"`
	DeletedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Team) TableName() string { return "teams" }
