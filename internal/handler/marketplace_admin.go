package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type adminUpdateMarketplaceAgentRequest struct {
	Featured *bool   `json:"featured,omitempty"`
	Popular  *bool   `json:"popular,omitempty"`
	Verified *bool   `json:"verified,omitempty"`
	Flagged  *bool   `json:"flagged,omitempty"`
	Status   *string `json:"status,omitempty"`
}

// AdminList handles GET /admin/v1/marketplace/agents.
// @Summary List all marketplace agents (admin)
// @Description Returns all marketplace agents regardless of status.
// @Tags admin
// @Produce json
// @Param status query string false "Filter by status"
// @Param flagged query bool false "Filter flagged agents"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[marketplaceAgentResponse]
// @Security BearerAuth
// @Router /admin/v1/marketplace/agents [get]
func (h *MarketplaceHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Preload("Publisher")
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if r.URL.Query().Get("flagged") == "true" {
		q = q.Where("flagged = true")
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.MarketplaceAgent
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list marketplace agents"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	resp := make([]marketplaceAgentResponse, len(agents))
	for i, agent := range agents {
		resp[i] = toMarketplaceAgentResponse(agent)
	}

	result := paginatedResponse[marketplaceAgentResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// AdminUpdate handles PUT /admin/v1/marketplace/agents/{id}.
// @Summary Admin update marketplace agent
// @Description Admin can set featured, popular, verified, flagged, and status.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Marketplace agent ID"
// @Param body body adminUpdateMarketplaceAgentRequest true "Fields to update"
// @Success 200 {object} marketplaceAgentResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/marketplace/agents/{id} [put]
func (h *MarketplaceHandler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var ma model.MarketplaceAgent
	if err := h.db.Where("id = ?", id).First(&ma).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "marketplace agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find marketplace agent"})
		return
	}

	var req adminUpdateMarketplaceAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Featured != nil {
		updates["featured"] = *req.Featured
	}
	if req.Popular != nil {
		updates["popular"] = *req.Popular
	}
	if req.Flagged != nil {
		updates["flagged"] = *req.Flagged
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Verified != nil {
		if *req.Verified {
			now := time.Now()
			updates["verified_at"] = &now
		} else {
			updates["verified_at"] = nil
		}
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	if err := h.db.Model(&ma).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update marketplace agent"})
		return
	}

	h.db.Preload("Publisher").Where("id = ?", id).First(&ma)
	slog.Info("admin: marketplace agent updated", "marketplace_id", ma.ID)
	writeJSON(w, http.StatusOK, toMarketplaceAgentResponse(ma))
}

// AdminDelete handles DELETE /admin/v1/marketplace/agents/{id}.
// @Summary Admin delete marketplace agent
// @Description Permanently deletes a marketplace agent.
// @Tags admin
// @Produce json
// @Param id path string true "Marketplace agent ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/marketplace/agents/{id} [delete]
func (h *MarketplaceHandler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result := h.db.Where("id = ?", id).Delete(&model.MarketplaceAgent{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete marketplace agent"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "marketplace agent not found"})
		return
	}

	slog.Info("admin: marketplace agent deleted", "marketplace_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// BustCache handles POST /admin/v1/marketplace/cache/bust.
// @Summary Bust marketplace cache
// @Description Flushes all marketplace cache keys from Redis.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]string
// @Security BearerAuth
// @Router /admin/v1/marketplace/cache/bust [post]
func (h *MarketplaceHandler) BustCache(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var deleted int64
	iter := h.redis.Scan(ctx, 0, marketplaceCachePrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		h.redis.Del(ctx, iter.Val())
		deleted++
	}

	slog.Info("admin: marketplace cache busted", "keys_deleted", deleted)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "keys_deleted": fmt.Sprintf("%d", deleted)})
}
