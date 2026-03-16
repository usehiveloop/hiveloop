package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ConnectSession represents a short-lived, scoped session for the Connect UI widget.
type ConnectSession struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org              Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IdentityID       *uuid.UUID     `gorm:"type:uuid;index"`
	Identity         *Identity      `gorm:"foreignKey:IdentityID;constraint:OnDelete:SET NULL"`
	ExternalID       string         `gorm:"column:external_id;default:''"`
	SessionToken     string         `gorm:"not null;uniqueIndex"`
	AllowedIntegrations pq.StringArray `gorm:"type:text[];column:allowed_integrations"`
	Permissions      pq.StringArray `gorm:"type:text[]"`
	AllowedOrigins   pq.StringArray `gorm:"type:text[]"`
	Metadata         JSON           `gorm:"type:jsonb;default:'{}'"`
	ActivatedAt      *time.Time
	ExpiresAt        time.Time `gorm:"not null"`
	CreatedAt        time.Time
}

func (ConnectSession) TableName() string { return "connect_sessions" }

// GenerateSessionToken produces a csess_ prefixed opaque token (32 random bytes, hex-encoded).
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}
	return "csess_" + hex.EncodeToString(b), nil
}
