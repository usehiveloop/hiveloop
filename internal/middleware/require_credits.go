package middleware

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// BalanceChecker is the minimal contract RequireCredits needs from the
// billing ledger. An interface (not *billing.CreditsService) keeps tests
// isolated: a fake implementation is trivial to wire up.
type BalanceChecker interface {
	Balance(orgID uuid.UUID) (int64, error)
}

// RequireCredits gates proxy requests on credit balance.
//
// It runs after TokenAuth in the proxy middleware chain. For BYOK calls
// (claims.IsSystem == false) it's a no-op — those don't consume credits for
// inference. For platform-keys calls it rejects with HTTP 402 Payment
// Required when the org's balance is at zero.
//
// The check is coarse (balance > 0, not balance > estimate): if a request
// slips through while the balance is razor-thin, the post-deduct task may
// drive it slightly negative. The next call then 402s cleanly.
func RequireCredits(credits BalanceChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				// TokenAuth should have populated claims; missing claims
				// means the middleware order is wrong. Fail closed.
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "missing auth claims",
				})
				return
			}

			// BYOK requests don't burn credits for inference — platform
			// infra (sandbox time) is metered separately, outside this path.
			if !claims.IsSystem {
				next.ServeHTTP(w, r)
				return
			}

			orgID, err := uuid.Parse(claims.OrgID)
			if err != nil {
				slog.Error("require_credits: invalid org id in claims",
					"org_id", claims.OrgID, "error", err)
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "invalid org id",
				})
				return
			}

			balance, err := credits.Balance(orgID)
			if err != nil {
				// Balance lookup failure is a DB error — fail closed
				// rather than gifting free inference.
				slog.Error("require_credits: balance lookup failed",
					"org_id", orgID, "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "failed to read credit balance",
				})
				return
			}

			if balance <= 0 {
				writeJSON(w, http.StatusPaymentRequired, map[string]string{
					"error":   "insufficient credits",
					"balance": "0",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
