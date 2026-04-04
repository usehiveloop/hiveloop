package model

import (
	"time"

	"github.com/google/uuid"
)

// OAuthAccount links a user to an external OAuth provider (e.g. GitHub, Google).
type OAuthAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_oauth_user_provider"`
	Provider       string    `gorm:"not null;uniqueIndex:idx_oauth_provider_uid;uniqueIndex:idx_oauth_user_provider"`
	ProviderUserID string    `gorm:"not null;uniqueIndex:idx_oauth_provider_uid"`
	User           User      `gorm:"foreignKey:UserID"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (OAuthAccount) TableName() string { return "oauth_accounts" }
