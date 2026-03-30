package auth

import (
	"crypto/rsa"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ValidateAccessToken parses and validates an RS256-signed access token.
func ValidateAccessToken(pubKey *rsa.PublicKey, issuer, audience, tokenStr string) (*AuthClaims, error) {
	parser := jwt.NewParser(
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)

	token, err := parser.ParseWithClaims(tokenStr, &AuthClaims{}, func(token *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("validating access token: %w", err)
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ValidateRefreshToken parses and validates an HS256-signed refresh token.
// Returns the user ID and JTI from the token.
func ValidateRefreshToken(hmacKey []byte, tokenStr string) (userID, jti string, err error) {
	parser := jwt.NewParser(
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"HS256"}),
	)

	token, err := parser.ParseWithClaims(tokenStr, &RefreshClaims{}, func(token *jwt.Token) (any, error) {
		return hmacKey, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("validating refresh token: %w", err)
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid refresh token claims")
	}

	return claims.UserID, claims.ID, nil
}
