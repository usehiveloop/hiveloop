package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type updateMarketplaceAgentRequest struct {
	Name         *string  `json:"name,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Avatar       *string  `json:"avatar,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Status       *string  `json:"status,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
}

// Update handles PUT /v1/marketplace/agents/{id}.
// @Summary Update a marketplace agent
// @Description Updates name, description, avatar, tags, instructions, or status. Only the publisher can update.
// @Tags marketplace
// @Accept json
// @Produce json
// @Param id path string true "Marketplace agent ID"
// @Param body body updateMarketplaceAgentRequest true "Fields to update"
// @Success 200 {object} marketplaceAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/marketplace/agents/{id} [put]
func (h *MarketplaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

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

	if ma.PublisherID != user.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the publisher can update this listing"})
		return
	}

	var req updateMarketplaceAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if req.Instructions != nil {
		updates["instructions"] = *req.Instructions
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.Status != nil {
		status := *req.Status
		if status != "draft" && status != "published" && status != "archived" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be draft, published, or archived"})
			return
		}
		updates["status"] = status
		if status == "published" && ma.PublishedAt == nil {
			now := time.Now()
			updates["published_at"] = &now
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
	slog.Info("marketplace agent updated", "marketplace_id", ma.ID, "publisher_id", user.ID)
	writeJSON(w, http.StatusOK, toMarketplaceAgentResponse(ma))
}

// Delete handles DELETE /v1/marketplace/agents/{id}.
// @Summary Remove a marketplace listing
// @Description Deletes a marketplace agent. Only the publisher can delete.
// @Tags marketplace
// @Produce json
// @Param id path string true "Marketplace agent ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/marketplace/agents/{id} [delete]
func (h *MarketplaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

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

	if ma.PublisherID != user.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the publisher can delete this listing"})
		return
	}

	if err := h.db.Delete(&ma).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete marketplace agent"})
		return
	}

	slog.Info("marketplace agent deleted", "marketplace_id", ma.ID, "publisher_id", user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
