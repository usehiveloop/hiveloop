package token

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProxyToken holds the result of MintAndPersist.
type ProxyToken struct {
	TokenString string    // "ptok_" prefixed JWT
	JTI         string    // unique token identifier
	ExpiresAt   time.Time // when the token expires
}

// MintAndPersist mints a proxy token, persists it to the database, and returns
// the prefixed token string. This is the canonical way to create proxy tokens.
//
// The meta parameter is stored in the token's JSONB meta field and should
// contain at minimum a "type" key (e.g. "agent_proxy", "embedding_proxy").
func MintAndPersist(db *gorm.DB, signingKey []byte, orgID, credentialID uuid.UUID, ttl time.Duration, meta map[string]any) (*ProxyToken, error) {
	tokenStr, jti, err := Mint(signingKey, orgID.String(), credentialID.String(), ttl)
	if err != nil {
		return nil, fmt.Errorf("minting token: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	jsonMeta := make(map[string]any, len(meta))
	for key, value := range meta {
		jsonMeta[key] = value
	}

	// Persist so the proxy middleware can validate by JTI
	if err := db.Exec(`
		INSERT INTO tokens (org_id, credential_id, jti, expires_at, meta, created_at)
		VALUES (?, ?, ?, ?, ?::jsonb, ?)
	`, orgID, credentialID, jti, expiresAt, toJSON(jsonMeta), now).Error; err != nil {
		return nil, fmt.Errorf("persisting token: %w", err)
	}

	return &ProxyToken{
		TokenString: "ptok_" + tokenStr,
		JTI:         jti,
		ExpiresAt:   expiresAt,
	}, nil
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Claims represents the JWT claims for a sandbox proxy token.
type Claims struct {
	OrgID        string `json:"org_id"`
	CredentialID string `json:"cred_id"`
	ScopeHash    string `json:"scope_hash,omitempty"`
	// IsSystem is true when the referenced credential is platform-owned
	// (credentials.is_system = true). Baked into the JWT at mint time so
	// the proxy can decide whether to gate on credit balance and meter
	// token spend without a DB round-trip.
	IsSystem bool `json:"is_system,omitempty"`
	jwt.RegisteredClaims
}

// MintOptions holds optional parameters for token minting.
type MintOptions struct {
	ScopeHash string // SHA-256 hash of scope rules, if scopes are present
	IsSystem  bool   // true when the credential is platform-owned
}

// Mint creates a signed JWT with the given claims and TTL.
// Returns the token string and the JTI.
func Mint(signingKey []byte, orgID, credentialID string, ttl time.Duration, opts ...MintOptions) (string, string, error) {
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

	if len(opts) > 0 {
		if opts[0].ScopeHash != "" {
			claims.ScopeHash = opts[0].ScopeHash
		}
		claims.IsSystem = opts[0].IsSystem
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
