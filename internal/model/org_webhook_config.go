package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type OrgWebhookConfig struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID           uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Org             Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	URL             string    `gorm:"not null"`
	EncryptedSecret []byte    `gorm:"type:bytea;not null"`
	SecretPrefix    string    `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (OrgWebhookConfig) TableName() string { return "org_webhook_configs" }

// GenerateWebhookSecret creates a new webhook signing secret.
// Returns the plaintext (shown once) and a display prefix.
func GenerateWebhookSecret() (plaintext, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating webhook secret: %w", err)
	}
	raw := hex.EncodeToString(b)
	plaintext = "zira_whs_" + raw
	prefix = plaintext[:17] // "zira_whs_" + first 8 hex chars
	return plaintext, prefix, nil
}
