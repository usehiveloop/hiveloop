package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT claims for a sandbox proxy token.
type Claims struct {
	OrgID        string `json:"org_id"`
	CredentialID string `json:"cred_id"`
	jwt.RegisteredClaims
}

// Mint creates a signed JWT with the given claims and TTL.
// Returns the token string and the JTI.
func Mint(signingKey []byte, orgID, credentialID string, ttl time.Duration) (string, string, error) {
	jti := uuid.New().String()
	now := time.Now()

	claims := Claims{
		OrgID:        orgID,
		CredentialID: credentialID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(signingKey)
	if err != nil {
		return "", "", fmt.Errorf("signing token: %w", err)
	}

	return tokenString, jti, nil
}

// Validate parses and validates a JWT, returning the claims if valid.
func Validate(signingKey []byte, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	if claims.OrgID == "" {
		return nil, fmt.Errorf("missing org_id claim")
	}
	if claims.CredentialID == "" {
		return nil, fmt.Errorf("missing cred_id claim")
	}

	return claims, nil
}
