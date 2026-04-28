package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type verifyRequest struct {
	PlanSlug string `json:"plan_slug"`
}

type verifyResponse struct {
	Status   string `json:"status"` // "active" | "timeout"
	PlanSlug string `json:"plan_slug,omitempty"`
}

// Verify polls the local DB for a Subscription matching (org, plan_slug,
// status=active). Returns as soon as one appears or after a 5s timeout.
//
// Used by the popup checkout flow: the browser fires this immediately after
// the popup's onSuccess callback, and the response confirms the
// subscription.create / charge.success webhook landed and our state has
// caught up. Webhooks are still the source of truth — this endpoint just
// surfaces "is it ready to render?" to the client.
//
// @Summary Verify subscription is active
// @Description Waits up to 5s for the subscription webhook to land and the local Subscription row to become active for the requested plan.
// @Tags billing
// @Accept json
// @Produce json
// @Param body body verifyRequest true "Plan to wait for"
// @Success 200 {object} verifyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 408 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/verify [post]
func (h *BillingHandler) Verify(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.PlanSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_slug is required"})
		return
	}

	deadline := time.Now().Add(5 * time.Second)
	tick := 300 * time.Millisecond
	ctx := r.Context()

	for {
		var sub model.Subscription
		err := h.db.
			Joins("JOIN plans ON plans.id = subscriptions.plan_id").
			Where("subscriptions.org_id = ? AND plans.slug = ? AND subscriptions.status = ?",
				org.ID, body.PlanSlug, string(billing.StatusActive)).
			Order("subscriptions.created_at DESC").
			First(&sub).Error
		if err == nil {
			writeJSON(w, http.StatusOK, verifyResponse{Status: "active", PlanSlug: body.PlanSlug})
			return
		}
		if err != gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query subscription"})
			return
		}

		if time.Now().After(deadline) {
			writeJSON(w, http.StatusRequestTimeout, verifyResponse{Status: "timeout", PlanSlug: body.PlanSlug})
			return
		}

		select {
		case <-ctx.Done():
			writeJSON(w, http.StatusRequestTimeout, verifyResponse{Status: "timeout", PlanSlug: body.PlanSlug})
			return
		case <-time.After(tick):
		}
	}
}
