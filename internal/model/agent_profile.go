package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentProfile is how an AI employee shows up on an external platform.
// Owned by exactly one employee agent (agent.is_employee = true).
type AgentProfile struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Org     Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID uuid.UUID `gorm:"type:uuid;not null;index"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`

	Provider   string `gorm:"not null;index"`
	ExternalID string `gorm:"not null;default:''"`
	Label      string `gorm:"not null;default:''"`

	Identity JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Config   JSON `gorm:"type:jsonb;not null;default:'{}'"`

	EncryptedIdentity []byte `gorm:"type:bytea"`
	EncryptedSecrets  []byte `gorm:"type:bytea"`
	WrappedDEK        []byte `gorm:"type:bytea"`

	Status         string `gorm:"not null;default:'active';index"`
	StatusReason   string `gorm:"not null;default:''"`
	LastVerifiedAt *time.Time

	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
}

func (AgentProfile) TableName() string { return "agent_profiles" }
