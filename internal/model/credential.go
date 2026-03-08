package model

import (
	"time"

	"github.com/google/uuid"
)

type Credential struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org          Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Label        string     `gorm:"not null;default:''"`
	BaseURL      string     `gorm:"not null"`
	AuthScheme   string     `gorm:"not null"`
	EncryptedKey []byte     `gorm:"type:bytea;not null"`
	WrappedDEK   []byte     `gorm:"type:bytea;not null"`
	RevokedAt    *time.Time
	CreatedAt    time.Time
}

func (Credential) TableName() string { return "credentials" }
