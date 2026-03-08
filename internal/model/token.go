package model

import (
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID        uuid.UUID  `gorm:"type:uuid;not null"`
	Org          Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	CredentialID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Credential   Credential `gorm:"foreignKey:CredentialID;constraint:OnDelete:CASCADE"`
	JTI          string     `gorm:"column:jti;not null;uniqueIndex"`
	ExpiresAt    time.Time  `gorm:"not null"`
	RevokedAt    *time.Time
	CreatedAt    time.Time
}

func (Token) TableName() string { return "tokens" }
