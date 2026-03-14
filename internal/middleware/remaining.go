package middleware

import (
	"log/slog"
	"net/http"

	"github.com/llmvault/llmvault/internal/counter"
)

// RemainingCheck returns middleware that enforces request caps on both the token
// and credential counters. It decrements the token counter first, then the
// credential counter. If the credential counter rejects, the token decrement is
// rolled back (INCR). If Redis is unavailable, the request is allowed through
// (fail-open). Returns 429 when a cap is exhausted with lazy refill attempted.
func RemainingCheck(ctr *counter.Counter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			// 1. Token counter
			tokResult, err := ctr.Decrement(ctx, counter.TokKey(claims.JTI))
			if err != nil {
				slog.Warn("remaining: redis error on token decrement, failing open", "jti", claims.JTI, "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if tokResult == counter.DecrExhausted {
				// Try lazy refill
				refilled, err := ctr.CheckAndRefillToken(ctx, claims.JTI)
				if err != nil {
					slog.Warn("remaining: token refill error", "jti", claims.JTI, "error", err)
				}
				if refilled {
					// Retry decrement after refill
					tokResult, err = ctr.Decrement(ctx, counter.TokKey(claims.JTI))
					if err != nil {
						slog.Warn("remaining: redis error after token refill", "jti", claims.JTI, "error", err)
						next.ServeHTTP(w, r)
						return
					}
				}
				if tokResult == counter.DecrExhausted {
					writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "token request cap exhausted"})
					return
				}
			}

			// 2. Credential counter
			credResult, err := ctr.Decrement(ctx, counter.CredKey(claims.CredentialID))
			if err != nil {
				slog.Warn("remaining: redis error on credential decrement, failing open", "credential_id", claims.CredentialID, "error", err)
				// Undo token decrement if we can't check credential
				if tokResult == counter.DecrOK {
					_ = ctr.Undo(ctx, counter.TokKey(claims.JTI))
				}
				next.ServeHTTP(w, r)
				return
			}
			if credResult == counter.DecrExhausted {
				// Try lazy refill
				refilled, err := ctr.CheckAndRefillCredential(ctx, claims.CredentialID)
				if err != nil {
					slog.Warn("remaining: credential refill error", "credential_id", claims.CredentialID, "error", err)
				}
				if refilled {
					credResult, err = ctr.Decrement(ctx, counter.CredKey(claims.CredentialID))
					if err != nil {
						slog.Warn("remaining: redis error after credential refill", "credential_id", claims.CredentialID, "error", err)
						if tokResult == counter.DecrOK {
							_ = ctr.Undo(ctx, counter.TokKey(claims.JTI))
						}
						next.ServeHTTP(w, r)
						return
					}
				}
				if credResult == counter.DecrExhausted {
					// Undo token decrement
					if tokResult == counter.DecrOK {
						_ = ctr.Undo(ctx, counter.TokKey(claims.JTI))
					}
					writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "credential request cap exhausted"})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
