package model

import (
	"time"

	"github.com/google/uuid"
)

type Sandbox struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID        `gorm:"type:uuid;not null;index"`
	Org               Org              `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IdentityID        uuid.UUID        `gorm:"type:uuid;not null;index:idx_sandbox_identity_type"`
	Identity          Identity         `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
	SandboxType       string           `gorm:"not null;index:idx_sandbox_identity_type"` // "shared" or "dedicated"
	AgentID           *uuid.UUID       `gorm:"type:uuid;index"`
	Agent             *Agent           `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`
	ExternalID        string           `gorm:"not null"`             // Daytona workspace ID
	BridgeURL         string           `gorm:"not null"`             // pre-authenticated URL to reach Bridge
	BridgeURLExpiresAt *time.Time                                    // when BridgeURL expires (nil = never)
	EncryptedBridgeAPIKey []byte       `gorm:"type:bytea;not null"`  // AES-256-GCM encrypted Bridge API key
	Status            string           `gorm:"not null;default:'creating'"` // creating, running, stopped, starting, error
	ErrorMessage      *string
	LastActiveAt      *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Sandbox) TableName() string { return "sandboxes" }
