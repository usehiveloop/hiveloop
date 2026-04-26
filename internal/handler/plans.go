package handler

import (
	"log/slog"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type PlansHandler struct {
	db *gorm.DB
}

func NewPlansHandler(db *gorm.DB) *PlansHandler {
	return &PlansHandler{db: db}
}

// List handles GET /v1/plans (no auth).
// @Summary List all active plans
// @Description Returns the public catalog of billing plans, ordered by price
// @Description ascending (free first). Public — no authentication required.
// @Tags plans
// @Produce json
// @Success 200 {array} planDTO
// @Failure 500 {object} errorResponse
// @Router /v1/plans [get]
func (h *PlansHandler) List(w http.ResponseWriter, r *http.Request) {
	var plans []model.Plan
	if err := h.db.Where("active = ?", true).
		Order("price_cents ASC, slug ASC").
		Find(&plans).Error; err != nil {
		slog.Error("plans list: query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plans"})
		return
	}

	out := make([]planDTO, 0, len(plans))
	for _, p := range plans {
		out = append(out, *planFromModel(p))
	}
	writeJSON(w, http.StatusOK, out)
}
