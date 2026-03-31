package middleware

import (
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
)

// TokenAuth returns middleware that authenticates requests using sandbox proxy tokens (JWTs).
//
// It extracts the token from whichever auth scheme the LLM provider uses:
//
//   - Authorization: Bearer ptok_...  (OpenAI, Groq, Mistral, etc.)
//   - x-api-key: ptok_...            (Anthropic)
//   - api-key: ptok_...              (Azure)
//   - ?key=ptok_...                  (Google, query parameter)
//
// The JWT is validated, then checked against the database for revocation.
// If valid, TokenClaims are placed on the request context.
func TokenAuth(signingKey []byte, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken := extractProxyToken(r)
			if rawToken == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
				return
			}

			// Strip the ptok_ prefix if present
			jwtString, _ := strings.CutPrefix(rawToken, "ptok_")

			claims, err := token.Validate(signingKey, jwtString)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
				return
			}

			// Check if the token has been revoked
			var tokenRecord model.Token
			result := db.Where("jti = ?", claims.ID).First(&tokenRecord)
			if result.Error != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not found"})
				return
			}

			if tokenRecord.RevokedAt != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token has been revoked"})
				return
			}

			tc := &TokenClaims{
				OrgID:        claims.OrgID,
				CredentialID: claims.CredentialID,
				JTI:          claims.ID,
			}

			next.ServeHTTP(w, WithClaims(r, tc))
		})
	}
}

// extractProxyToken extracts the proxy token from the request, checking
// all auth schemes that LLM providers use:
//
//  1. Authorization: Bearer ptok_...  (OpenAI and compatible)
//  2. x-api-key: ptok_...            (Anthropic)
//  3. api-key: ptok_...              (Azure)
//  4. ?key=ptok_...                  (Google query param)
func extractProxyToken(r *http.Request) string {
	// 1. Authorization: Bearer
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}

	// 2. x-api-key (Anthropic)
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}

	// 3. api-key (Azure)
	if key := r.Header.Get("api-key"); key != "" {
		return key
	}

	// 4. ?key= query parameter (Google)
	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}

	return ""
}
