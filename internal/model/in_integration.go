package model

import (
	"time"

	"github.com/google/uuid"
)

type InIntegration struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UniqueKey   string     `gorm:"not null;uniqueIndex"`
	Provider    string     `gorm:"not null;uniqueIndex:idx_in_integ_provider"`
	DisplayName string     `gorm:"not null"`
	Meta        JSON       `gorm:"type:jsonb;default:'{}'"`
	NangoConfig JSON       `gorm:"type:jsonb;default:'{}'" json:"nango_config"`
	// SupportsRAGSource is the admin-UI picker gate: only integrations with
	// this flag true appear in "Add RAG source". Seeded for the Phase 3
	// allowlist (github, notion, slack, confluence, jira, linear,
	// google_drive) by ragmodel.AutoMigrate3A; anything else defaults false.
	SupportsRAGSource bool       `gorm:"not null;default:false" json:"supports_rag_source"`
	DeletedAt         *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (InIntegration) TableName() string { return "in_integrations" }
