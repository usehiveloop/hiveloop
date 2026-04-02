package model

import (
	"time"

	"github.com/google/uuid"
)

// HindsightBank tracks which identities have had their Hindsight memory bank
// created and configured. Used for lazy bank provisioning and config change detection.
type HindsightBank struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	IdentityID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Identity   Identity  `gorm:"foreignKey:IdentityID;constraint:OnDelete:CASCADE"`
	BankID     string    `gorm:"not null;uniqueIndex"` // "identity-{uuid}"
	ConfigHash string    `gorm:"not null;default:''"` // SHA256 of applied MemoryConfig
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (HindsightBank) TableName() string { return "hindsight_banks" }
