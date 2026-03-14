package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Org struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string         `gorm:"not null;uniqueIndex"`
	LogtoOrgID     string         `gorm:"not null;uniqueIndex"`
	RateLimit      int            `gorm:"not null;default:1000"`
	Active         bool           `gorm:"not null;default:true"`
	AllowedOrigins pq.StringArray `gorm:"type:text[]"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Org) TableName() string { return "orgs" }

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&Org{},
		&Identity{},
		&IdentityRateLimit{},
		&Credential{},
		&Token{},
		&AuditEntry{},
		&Usage{},
		&ConnectSession{},
		&APIKey{},
		&Integration{},
	); err != nil {
		return err
	}

	// GIN indexes for JSONB metadata filtering
	db.Exec("CREATE INDEX IF NOT EXISTS idx_credentials_meta ON credentials USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_tokens_meta ON tokens USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_identities_meta ON identities USING GIN (meta jsonb_path_ops)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_integrations_meta ON integrations USING GIN (meta jsonb_path_ops)")

	return nil
}
