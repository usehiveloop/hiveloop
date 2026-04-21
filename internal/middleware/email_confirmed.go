package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// RequireEmailConfirmed blocks JWT-authenticated users whose email is not yet
// confirmed. API key auth is not affected — only JWT-based (human) sessions
// are gated. This middleware must run after MultiAuth.
func RequireEmailConfirmed(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API key auth bypasses email confirmation.
			if _, ok := APIKeyClaimsFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			claims, ok := AuthClaimsFromContext(r.Context())
			if !ok {
				// No auth claims at all — let downstream middleware handle 401.
				next.ServeHTTP(w, r)
				return
			}

			var user model.User
			if err := db.Select("id", "email_confirmed_at", "banned_at").Where("id = ?", claims.UserID).First(&user).Error; err != nil {
				slog.Warn("email confirmed check: user not found", "user_id", claims.UserID, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "email_not_confirmed",
					"message": "Please confirm your email address before accessing this resource.",
				})
				return
			}

			if user.BannedAt != nil {
				slog.Warn("email confirmed check: account banned", "user_id", claims.UserID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "account_banned",
					"message": "Your account has been suspended.",
				})
				return
			}

			if user.EmailConfirmedAt == nil {
				slog.Warn("email confirmed check: email not confirmed", "user_id", claims.UserID)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "email_not_confirmed",
					"message": "Please confirm your email address before accessing this resource.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
