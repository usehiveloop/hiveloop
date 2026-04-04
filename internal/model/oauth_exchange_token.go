package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OAuthExchangeToken is a short-lived, single-use token that the backend issues
// after a successful OAuth callback. The frontend exchanges it for an access/refresh
// token pair via POST /oauth/exchange.
type OAuthExchangeToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	TokenHash string     `gorm:"not null;uniqueIndex"`
	ExpiresAt time.Time  `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (OAuthExchangeToken) TableName() string { return "oauth_exchange_tokens" }

// GenerateExchangeToken creates a 32-byte random token (hex-encoded) and its SHA-256 hash.
func GenerateExchangeToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating exchange token: %w", err)
	}
	plaintext = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// HashExchangeToken returns the SHA-256 hex digest of a plaintext token.
func HashExchangeToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
