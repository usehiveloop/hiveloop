package auth

import (
	"crypto/rsa"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthClaims represents the claims embedded in an HiveLoop access token.
type AuthClaims struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
	// ImpersonationOf, if non-empty, indicates this token was issued by a
	// platform admin impersonating the user identified by UserID. The value
	// is the admin user ID that initiated the impersonation session.
	ImpersonationOf string `json:"impersonation_of,omitempty"`
	jwt.RegisteredClaims
}

// RefreshClaims represents the claims embedded in a refresh token.
type RefreshClaims struct {
	UserID string `json:"user_id"`
	// ImpersonationOf, if non-empty, marks this refresh token as belonging
	// to an impersonation session initiated by the given admin user ID.
	ImpersonationOf string `json:"impersonation_of,omitempty"`
	jwt.RegisteredClaims
}

// IssueAccessToken creates an RS256-signed JWT access token scoped to a specific org.
func IssueAccessToken(key *rsa.PrivateKey, issuer, audience, userID, orgID, role string, ttl time.Duration) (string, error) {
	return IssueAccessTokenWithImpersonation(key, issuer, audience, userID, orgID, role, "", ttl)
}

// IssueAccessTokenWithImpersonation creates an RS256-signed JWT access token,
// optionally tagging it as an impersonation session via impersonationOf (the
// admin user ID).
func IssueAccessTokenWithImpersonation(key *rsa.PrivateKey, issuer, audience, userID, orgID, role, impersonationOf string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := AuthClaims{
		UserID:          userID,
		OrgID:           orgID,
		Role:            role,
		ImpersonationOf: impersonationOf,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}

// IssueRefreshToken creates an HS256-signed refresh token for token rotation.
func IssueRefreshToken(hmacKey []byte, userID string, ttl time.Duration) (string, error) {
	return IssueRefreshTokenWithImpersonation(hmacKey, userID, "", ttl)
}

// IssueRefreshTokenWithImpersonation creates an HS256-signed refresh token,
// optionally tagging it as belonging to an impersonation session.
func IssueRefreshTokenWithImpersonation(hmacKey []byte, userID, impersonationOf string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := RefreshClaims{
		UserID:          userID,
		ImpersonationOf: impersonationOf,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(hmacKey)
}
