package model

import (
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null"`
	Org            Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	CredentialID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	Credential     Credential `gorm:"foreignKey:CredentialID;constraint:OnDelete:CASCADE"`
	JTI            string     `gorm:"column:jti;not null;uniqueIndex"`
	ExpiresAt      time.Time  `gorm:"not null"`
	Remaining      *int64     `gorm:"column:remaining"`
	RefillAmount   *int64     `gorm:"column:refill_amount"`
	RefillInterval *string    `gorm:"column:refill_interval"`
	LastRefillAt   *time.Time `gorm:"column:last_refill_at"`
	Meta           JSON       `gorm:"type:jsonb;default:'{}'"`
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

func (Token) TableName() string { return "tokens" }
