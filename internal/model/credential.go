package model

import (
	"time"

	"github.com/google/uuid"
)

type Credential struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org            Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Label          string     `gorm:"not null;default:''"`
	BaseURL        string     `gorm:"not null"`
	AuthScheme     string     `gorm:"not null"`
	EncryptedKey   []byte     `gorm:"type:bytea;not null"`
	WrappedDEK     []byte     `gorm:"type:bytea;not null"`
	Remaining      *int64     `gorm:"column:remaining"`
	RefillAmount   *int64     `gorm:"column:refill_amount"`
	RefillInterval *string    `gorm:"column:refill_interval"`
	LastRefillAt   *time.Time `gorm:"column:last_refill_at"`
	ProviderID     string     `gorm:"column:provider_id;default:''"`
	Meta           JSON       `gorm:"type:jsonb;default:'{}'"`
	// IsSystem marks credentials owned by the platform itself rather than by
	// a customer org. System credentials are used by agents that opted out of
	// BYOK (agent.credential_id IS NULL), managed via admin-only endpoints,
	// and hidden from org-scoped APIs. They FK to the platform org (see
	// internal/credentials.PlatformOrgID).
	IsSystem  bool `gorm:"not null;default:false;index"`
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (Credential) TableName() string { return "credentials" }
