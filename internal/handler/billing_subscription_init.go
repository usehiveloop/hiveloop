package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/middleware"
)

type initUpgradeRequest struct {
	QuoteID string `json:"quote_id"`
}

type initUpgradeResponse struct {
	AccessCode  string `json:"access_code"`
	Reference   string `json:"reference"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

// @Summary Initialise a Paystack transaction for an upgrade quote
// @Tags billing
// @Accept json
// @Produce json
// @Param body body initUpgradeRequest true "Upgrade quote id"
// @Success 200 {object} initUpgradeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 410 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/subscription/init-upgrade [post]
func (h *SubscriptionHandler) InitUpgrade(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var body initUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.QuoteID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quote_id is required"})
		return
	}
	quoteID, err := uuid.Parse(body.QuoteID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quote_id is not a valid uuid"})
		return
	}

	init, err := h.service.InitUpgrade(r.Context(), org.ID, quoteID)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrQuoteUnknown):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown quote"})
		case errors.Is(err, subscription.ErrQuoteWrongOrg):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "quote belongs to another org"})
		case errors.Is(err, subscription.ErrQuoteExpired):
			writeJSON(w, http.StatusGone, map[string]string{"error": "quote has expired"})
		case errors.Is(err, subscription.ErrQuoteConsumed):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "quote already consumed"})
		case errors.Is(err, subscription.ErrNotAnUpgrade):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quote is not an upgrade"})
		default:
			slog.Error("init-upgrade: failed", "org_id", org.ID, "quote_id", quoteID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start upgrade"})
		}
		return
	}

	writeJSON(w, http.StatusOK, initUpgradeResponse{
		AccessCode:  init.AccessCode,
		Reference:   init.Reference,
		AmountMinor: init.AmountMinor,
		Currency:    init.Currency,
	})
}
