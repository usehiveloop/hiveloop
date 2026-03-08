package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Org struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name         string    `gorm:"not null"`
	ZitadelOrgID string    `gorm:"not null;uniqueIndex"`
	RateLimit    int       `gorm:"not null;default:1000"`
	Active       bool      `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Org) TableName() string { return "orgs" }

func AutoMigrate(db *gorm.DB) error {
	// Transition: make old api_key_hash column nullable (if it exists)
	db.Exec("ALTER TABLE orgs ALTER COLUMN api_key_hash DROP NOT NULL")
	db.Exec("DROP INDEX IF EXISTS idx_orgs_api_key_hash")

	return db.AutoMigrate(
		&Org{},
		&Credential{},
		&Token{},
		&AuditEntry{},
		&Usage{},
	)
}
