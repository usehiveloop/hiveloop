package middleware

import (
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/token"
)

// TokenAuth returns middleware that authenticates requests using sandbox proxy tokens (JWTs).
//
// It expects the Authorization header in the form "Bearer ptok_...".
// The JWT is validated, then checked against the database for revocation.
// If valid, TokenClaims are placed on the request context.
func TokenAuth(signingKey []byte, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization format"})
				return
			}

			rawToken := parts[1]

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
