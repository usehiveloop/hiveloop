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
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

func (Credential) TableName() string { return "credentials" }
