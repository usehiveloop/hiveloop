package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListConversations handles GET /admin/v1/conversations.
// @Summary List all conversations
// @Description Returns agent conversations across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param agent_id query string false "Filter by agent ID"
// @Param status query string false "Filter by status (active, ended, error)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminConversationResponse]
// @Security BearerAuth
// @Router /admin/v1/conversations [get]
func (h *AdminHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.AgentConversation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var conversations []model.AgentConversation
	if err := q.Find(&conversations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list conversations"})
		return
	}

	hasMore := len(conversations) > limit
	if hasMore {
		conversations = conversations[:limit]
	}

	resp := make([]adminConversationResponse, len(conversations))
	for i, c := range conversations {
		resp[i] = toAdminConversationResponse(c)
	}

	result := paginatedResponse[adminConversationResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := conversations[len(conversations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetConversation handles GET /admin/v1/conversations/{id}.
// @Summary Get conversation details
// @Description Returns conversation details with event count.
// @Tags admin
// @Produce json
// @Param id path string true "Conversation ID"
// @Success 200 {object} adminConversationResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/conversations/{id} [get]
func (h *AdminHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var conv model.AgentConversation
	if err := h.db.Where("id = ?", id).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get conversation"})
		return
	}

	var events []model.ConversationEvent
	h.db.Where("conversation_id = ?", conv.ID).Order("created_at ASC").Find(&events)

	type detailResponse struct {
		adminConversationResponse
		EventCount int `json:"event_count"`
	}

	writeJSON(w, http.StatusOK, detailResponse{
		adminConversationResponse: toAdminConversationResponse(conv),
		EventCount:                len(events),
	})
}

// EndConversation handles DELETE /admin/v1/conversations/{id}.
// @Summary End a conversation
// @Description Force-ends an active conversation.
// @Tags admin
// @Produce json
// @Param id path string true "Conversation ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/conversations/{id} [delete]
func (h *AdminHandler) EndConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.AgentConversation{}).
		Where("id = ? AND status = ?", id, "active").
		Updates(map[string]any{"status": "ended", "ended_at": now})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to end conversation"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found or not active"})
		return
	}

	slog.Info("admin: conversation ended", "conversation_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}
// ListGenerations handles GET /admin/v1/generations.
// @Summary List all generations
// @Description Returns LLM generations across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param provider_id query string false "Filter by provider ID"
// @Param model query string false "Filter by model name"
// @Param limit query int false "Page size"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/generations [get]
func (h *AdminHandler) ListGenerations(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			limit = n
		}
	}

	q := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if provider := r.URL.Query().Get("provider_id"); provider != "" {
		q = q.Where("provider_id = ?", provider)
	}
	if m := r.URL.Query().Get("model"); m != "" {
		q = q.Where("model = ?", m)
	}

	q = q.Order("created_at DESC").Limit(limit + 1)

	var gens []model.Generation
	if err := q.Find(&gens).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list generations"})
		return
	}

	hasMore := len(gens) > limit
	if hasMore {
		gens = gens[:limit]
	}

	resp := make([]adminGenerationResponse, len(gens))
	for i, g := range gens {
		resp[i] = toAdminGenerationResponse(g)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp, "has_more": hasMore})
}

// GenerationStats handles GET /admin/v1/generations/stats.
// @Summary Generation statistics
// @Description Returns aggregate generation statistics by provider and model.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Success 200 {object} adminGenerationStatsResponse
// @Security BearerAuth
// @Router /admin/v1/generations/stats [get]
func (h *AdminHandler) GenerationStats(w http.ResponseWriter, r *http.Request) {
	var stats adminGenerationStatsResponse

	q := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}

	var totals struct {
		Count  int64
		Cost   float64
		Input  int64
		Output int64
	}
	q.Select("COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input, COALESCE(SUM(output_tokens), 0) as output").Scan(&totals)

	stats.TotalGenerations = totals.Count
	stats.TotalCost = totals.Cost
	stats.TotalInput = totals.Input
	stats.TotalOutput = totals.Output

	// By provider
	q2 := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q2 = q2.Where("org_id = ?", orgID)
	}
	q2.Select("provider_id, COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input_tokens, COALESCE(SUM(output_tokens), 0) as output_tokens").
		Group("provider_id").Order("count DESC").Limit(20).Scan(&stats.ByProvider)

	// By model
	q3 := h.db.Model(&model.Generation{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q3 = q3.Where("org_id = ?", orgID)
	}
	q3.Select("model, COUNT(*) as count, COALESCE(SUM(cost), 0) as cost, COALESCE(SUM(input_tokens), 0) as input_tokens, COALESCE(SUM(output_tokens), 0) as output_tokens").
		Group("model").Order("count DESC").Limit(20).Scan(&stats.ByModel)

	if stats.ByProvider == nil {
		stats.ByProvider = []adminProviderStatEntry{}
	}
	if stats.ByModel == nil {
		stats.ByModel = []adminModelStatEntry{}
	}

	writeJSON(w, http.StatusOK, stats)
}