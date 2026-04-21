package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type APIKey struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org        Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name       string         `gorm:"not null"`
	KeyHash    string         `gorm:"not null;uniqueIndex"`
	KeyPrefix  string         `gorm:"not null"`
	Scopes     pq.StringArray `gorm:"type:text[];not null"`
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

func (APIKey) TableName() string { return "api_keys" }

// GenerateAPIKey creates a new API key with a plaintext value, its SHA-256 hash, and a display prefix.
func GenerateAPIKey() (plaintext, hash, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generating api key: %w", err)
	}
	raw := hex.EncodeToString(b)
	plaintext = "hvl_sk_" + raw
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	prefix = plaintext[:16] // "hvl_sk_" + first 9 hex chars
	return plaintext, hash, prefix, nil
}

// HashAPIKey returns the SHA-256 hex digest of a plaintext API key.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ValidAPIKeyScopes is the set of allowed scope values.
var ValidAPIKeyScopes = map[string]bool{
	"connect":      true,
	"credentials":  true,
	"tokens":       true,
	"integrations": true,
	"agents":       true,
	"all":          true,
}
