package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrgInvite is a pending invitation for a user (identified by email) to join an organization.
// The plaintext token is never stored — only its SHA-256 hash. The plaintext is delivered
// to the recipient exactly once via the invitation email.
type OrgInvite struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Email       string    `gorm:"not null;index"`
	Role        string    `gorm:"not null"` // "admin" | "member" | "viewer"
	TokenHash   string    `gorm:"not null;uniqueIndex"`
	InvitedByID uuid.UUID `gorm:"type:uuid;not null"`
	ExpiresAt   time.Time `gorm:"not null"`
	AcceptedAt  *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Org       Org  `gorm:"foreignKey:OrgID"`
	InvitedBy User `gorm:"foreignKey:InvitedByID"`
}

func (OrgInvite) TableName() string { return "org_invites" }

// GenerateInviteToken creates a 32-byte random token (hex-encoded) and its SHA-256 hash.
func GenerateInviteToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating invite token: %w", err)
	}
	plaintext = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// HashInviteToken returns the SHA-256 hex digest of a plaintext token.
func HashInviteToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
