package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/handler"
)

// mountBillingRoutes registers /v1/billing/* under the JWT-protected router.
// Kept in its own file so serve_routes_v1.go stays under the file-length cap.
func mountBillingRoutes(r chi.Router, billingHandler *handler.BillingHandler, subscriptionHandler *handler.SubscriptionHandler) {
	if billingHandler == nil {
		return
	}
	r.Post("/billing/checkout", billingHandler.CreateCheckout)
	r.Post("/billing/verify", billingHandler.Verify)
	r.Get("/billing/subscription", billingHandler.GetSubscription)

	r.Post("/billing/subscription/preview-change", subscriptionHandler.PreviewChange)
	r.Post("/billing/subscription/init-upgrade", subscriptionHandler.InitUpgrade)
	r.Post("/billing/subscription/apply-change", subscriptionHandler.ApplyChange)
	r.Post("/billing/subscription/cancel", subscriptionHandler.Cancel)
	r.Post("/billing/subscription/resume", subscriptionHandler.Resume)
}
