package model

import (
	"time"

	"github.com/google/uuid"
)

type InIntegration struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UniqueKey   string     `gorm:"not null;uniqueIndex"`
	Provider    string     `gorm:"not null;index"`
	DisplayName string     `gorm:"not null"`
	OrgID       *uuid.UUID `gorm:"type:uuid;index"`
	Org         *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID     *uuid.UUID `gorm:"type:uuid;index"`
	Agent       *Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	CustomApp   bool       `gorm:"not null;default:false;index"`
	Meta        JSON       `gorm:"type:jsonb;default:'{}'"`
	NangoConfig JSON       `gorm:"type:jsonb;default:'{}'" json:"nango_config"`
	ManagedBy   string     `gorm:"not null;default:'';index" json:"managed_by,omitempty"`
	ManagedID   string     `gorm:"not null;default:'';index" json:"managed_id,omitempty"`
	ManagedHash string     `gorm:"not null;default:''" json:"managed_hash,omitempty"`
	Required    bool       `gorm:"not null;default:false" json:"required"`
	// SupportsRAGSource is the admin-UI picker gate: only integrations
	// with this flag true appear in "Add RAG source". Seeded true for
	// the known-good providers (github, notion, slack, confluence,
	// jira, linear, google_drive) by the RAG model package's Migrate
	// entry point; anything else defaults false.
	SupportsRAGSource bool       `gorm:"not null;default:false" json:"supports_rag_source"`
	DeletedAt         *time.Time `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (InIntegration) TableName() string { return "in_integrations" }
