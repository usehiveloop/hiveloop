package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
)

type OTPCode struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email     string     `gorm:"not null;index"`
	TokenHash string     `gorm:"not null;uniqueIndex"`
	ExpiresAt time.Time  `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (OTPCode) TableName() string { return "otp_codes" }

// GenerateOTPCode creates a cryptographically secure 6-digit code and its SHA-256 hash.
func GenerateOTPCode() (plaintext, hash string, err error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", "", fmt.Errorf("generating OTP code: %w", err)
	}
	plaintext = fmt.Sprintf("%06d", n.Int64())
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// HashOTPCode returns the SHA-256 hex digest of a plaintext OTP code.
func HashOTPCode(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
