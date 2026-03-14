package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
)

// LogtoAuth validates Logto JWTs using JWKS fetched from the issuer.
type LogtoAuth struct {
	issuer   string // e.g. http://localhost:3001/oidc
	audience string // API resource indicator, e.g. https://api.llmvault.dev
	jwksURL  string // optional override for JWKS fetch URL (useful when issuer is not directly reachable)

	mu   sync.RWMutex
	jwks *jwksCache
}

type jwksCache struct {
	keys      map[string]json.RawMessage // kid -> JWK
	expiresAt time.Time
}

// NewLogtoAuth creates a JWT validation middleware that fetches JWKS from Logto.
// issuer is the OIDC issuer URL (e.g. http://localhost:3001/oidc).
// audience is the API resource indicator (e.g. https://api.llmvault.dev).
func NewLogtoAuth(issuer, audience string) *LogtoAuth {
	return &LogtoAuth{
		issuer:   strings.TrimRight(issuer, "/"),
		audience: audience,
	}
}

// SetJWKSURL overrides the URL used to fetch JWKS keys. By default, keys are
// fetched from {issuer}/jwks. Use this when the issuer URL (embedded in JWTs)
// differs from the URL where Logto is reachable (e.g., port mapping).
func (a *LogtoAuth) SetJWKSURL(url string) {
	a.jwksURL = strings.TrimRight(url, "/")
}

// LogtoClaims are the claims we extract from Logto JWTs.
type LogtoClaims struct {
	Sub            string `json:"sub"`
	OrganizationID string `json:"organization_id"`
	Scope          string `json:"scope"`
	ClientID       string `json:"client_id"`
}

type logtoClaimsKey struct{}

// LogtoClaimsFromContext retrieves Logto JWT claims from the request context.
func LogtoClaimsFromContext(ctx context.Context) (*LogtoClaims, bool) {
	claims, ok := ctx.Value(logtoClaimsKey{}).(*LogtoClaims)
	return claims, ok
}

// RequireAuthorization returns middleware that validates the Bearer JWT token.
func (a *LogtoAuth) RequireAuthorization() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization token"})
				return
			}

			claims, err := a.validateToken(r.Context(), tokenStr)
			if err != nil {
				slog.Warn("token validation failed", "error", err, "issuer", a.issuer, "audience", a.audience)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}

			ctx := context.WithValue(r.Context(), logtoClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveOrg returns middleware that extracts the organization_id from the JWT,
// loads the corresponding Org from the database, and sets it on the context.
func ResolveOrg(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := LogtoClaimsFromContext(r.Context())
			if !ok || claims.OrganizationID == "" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
				return
			}

			var org model.Org
			if err := db.Where("logto_org_id = ?", claims.OrganizationID).First(&org).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization not found"})
				return
			}

			if !org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			next.ServeHTTP(w, WithOrg(r, &org))
		})
	}
}

// RequireScope returns middleware that checks if the JWT has the required scope.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := LogtoClaimsFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
				return
			}

			scopes := strings.Fields(claims.Scope)
			for _, s := range scopes {
				if s == scope {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

// validateToken parses and validates a JWT against the Logto JWKS.
func (a *LogtoAuth) validateToken(_ context.Context, tokenStr string) (*LogtoClaims, error) {
	// Parse unverified to get the kid
	parser := jwt.NewParser(
		jwt.WithIssuer(a.issuer),
		jwt.WithAudience(a.audience),
		jwt.WithExpirationRequired(),
	)

	token, err := parser.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		// Ensure RSA or EC signing method
		switch token.Method.(type) {
		case *jwt.SigningMethodRSA, *jwt.SigningMethodECDSA:
			// ok
		default:
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid in token header")
		}

		return a.getKey(kid, token.Method.Alg())
	})
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unexpected claims type")
	}

	claims := &LogtoClaims{}
	claims.Sub, _ = mapClaims["sub"].(string)
	claims.OrganizationID, _ = mapClaims["organization_id"].(string)
	claims.Scope, _ = mapClaims["scope"].(string)
	claims.ClientID, _ = mapClaims["client_id"].(string)

	return claims, nil
}

// getKey returns the public key for the given kid by fetching JWKS from Logto.
func (a *LogtoAuth) getKey(kid, alg string) (any, error) {
	a.mu.RLock()
	if a.jwks != nil && time.Now().Before(a.jwks.expiresAt) {
		if keyData, ok := a.jwks.keys[kid]; ok {
			a.mu.RUnlock()
			return parseJWK(keyData, alg)
		}
	}
	a.mu.RUnlock()

	// Refresh JWKS
	if err := a.refreshJWKS(); err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	keyData, ok := a.jwks.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return parseJWK(keyData, alg)
}

func (a *LogtoAuth) refreshJWKS() error {
	jwksEndpoint := a.issuer + "/jwks"
	if a.jwksURL != "" {
		jwksEndpoint = a.jwksURL + "/jwks"
	}
	resp, err := http.Get(jwksEndpoint)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading JWKS: %w", err)
	}

	var jwksResp struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return fmt.Errorf("parsing JWKS: %w", err)
	}

	keys := make(map[string]json.RawMessage, len(jwksResp.Keys))
	for _, rawKey := range jwksResp.Keys {
		var key struct {
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(rawKey, &key); err != nil {
			continue
		}
		keys[key.Kid] = rawKey
	}

	a.mu.Lock()
	a.jwks = &jwksCache{
		keys:      keys,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	a.mu.Unlock()

	return nil
}

// parseJWK converts a raw JWK JSON into a crypto.PublicKey usable for verification.
func parseJWK(raw json.RawMessage, alg string) (any, error) {
	var key struct {
		Kty string `json:"kty"`
		// RSA
		N string `json:"n"`
		E string `json:"e"`
		// EC
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, fmt.Errorf("parsing JWK: %w", err)
	}

	switch key.Kty {
	case "RSA":
		return parseRSAPublicKey(key.N, key.E)
	case "EC":
		return parseECPublicKey(key.Crv, key.X, key.Y)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", key.Kty)
	}
}
