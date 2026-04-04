package model

import (
	"time"

	"github.com/google/uuid"
)

type Identity struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_identity_org_external"`
	Org        Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ExternalID string    `gorm:"not null;uniqueIndex:idx_identity_org_external"`
	Meta         JSON      `gorm:"type:jsonb;default:'{}'"`
	MemoryConfig JSON      `gorm:"type:jsonb;default:'{}'"` // Hindsight memory configuration
	CreatedAt    time.Time
	UpdatedAt  time.Time

	RateLimits []IdentityRateLimit `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
}

func (Identity) TableName() string { return "identities" }

type IdentityRateLimit struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	IdentityID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_identity_ratelimit_name"`
	Name       string    `gorm:"not null;uniqueIndex:idx_identity_ratelimit_name"`
	Limit      int64     `gorm:"not null"`
	Duration   int64     `gorm:"not null"` // milliseconds
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (IdentityRateLimit) TableName() string { return "identity_rate_limits" }
