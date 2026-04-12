package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

const (
	marketplaceCacheTTL    = 5 * time.Minute
	marketplaceCachePrefix = "marketplace:"
)

// MarketplaceHandler manages the agent marketplace.
type MarketplaceHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewMarketplaceHandler creates a new marketplace handler.
func NewMarketplaceHandler(db *gorm.DB, redisClient *redis.Client) *MarketplaceHandler {
	return &MarketplaceHandler{db: db, redis: redisClient}
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type createMarketplaceAgentRequest struct {
	AgentID string `json:"agent_id"`
}

type updateMarketplaceAgentRequest struct {
	Name         *string  `json:"name,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Avatar       *string  `json:"avatar,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Status       *string  `json:"status,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
}

type marketplaceAgentResponse struct {
	ID                   string   `json:"id"`
	Slug                 string   `json:"slug"`
	Name                 string   `json:"name"`
	Description          *string  `json:"description,omitempty"`
	Avatar               *string  `json:"avatar,omitempty"`
	SystemPrompt         string   `json:"system_prompt"`
	Instructions         *string  `json:"instructions,omitempty"`
	Model                string   `json:"model"`
	SandboxType          string   `json:"sandbox_type"`
	Tools                model.JSON `json:"tools"`
	McpServers           model.JSON `json:"mcp_servers"`
	Skills               model.JSON `json:"skills"`
	Integrations         model.JSON `json:"integrations"`
	AgentConfig          model.JSON `json:"agent_config"`
	Permissions          model.JSON `json:"permissions"`
	Team                 string   `json:"team"`
	SharedMemory         bool     `json:"shared_memory"`
	RequiredIntegrations []string `json:"required_integrations"`
	Tags                 []string `json:"tags"`
	Status               string   `json:"status"`
	Featured             bool     `json:"featured"`
	Popular              bool     `json:"popular"`
	Verified             bool     `json:"verified"`
	Flagged              bool     `json:"flagged"`
	InstallCount         int      `json:"install_count"`
	PublisherID          string   `json:"publisher_id"`
	PublisherName        string   `json:"publisher_name,omitempty"`
	SourceAgentID        *string  `json:"source_agent_id,omitempty"`
	PublishedAt          *string  `json:"published_at,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

func toMarketplaceAgentResponse(ma model.MarketplaceAgent) marketplaceAgentResponse {
	resp := marketplaceAgentResponse{
		ID:                   ma.ID.String(),
		Slug:                 ma.Slug,
		Name:                 ma.Name,
		Description:          ma.Description,
		Avatar:               ma.Avatar,
		SystemPrompt:         ma.SystemPrompt,
		Instructions:         ma.Instructions,
		Model:                ma.Model,
		SandboxType:          ma.SandboxType,
		Tools:                ma.Tools,
		McpServers:           ma.McpServers,
		Skills:               ma.Skills,
		Integrations:         ma.Integrations,
		AgentConfig:          ma.AgentConfig,
		Permissions:          ma.Permissions,
		Team:                 ma.Team,
		SharedMemory:         ma.SharedMemory,
		RequiredIntegrations: ma.RequiredIntegrations,
		Tags:                 ma.Tags,
		Status:               ma.Status,
		Featured:             ma.Featured,
		Popular:              ma.Popular,
		Verified:             ma.VerifiedAt != nil,
		Flagged:              ma.Flagged,
		InstallCount:         ma.InstallCount,
		PublisherID:          ma.PublisherID.String(),
		PublisherName:        ma.Publisher.Name,
		CreatedAt:            ma.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            ma.UpdatedAt.Format(time.RFC3339),
	}
	if ma.SourceAgentID != nil {
		s := ma.SourceAgentID.String()
		resp.SourceAgentID = &s
	}
	if ma.PublishedAt != nil {
		s := ma.PublishedAt.Format(time.RFC3339)
		resp.PublishedAt = &s
	}
	if resp.RequiredIntegrations == nil {
		resp.RequiredIntegrations = []string{}
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	return resp
}

// ---------------------------------------------------------------------------
// Public endpoints (no auth, cached)
// ---------------------------------------------------------------------------

// List handles GET /v1/marketplace/agents.
// @Summary List published marketplace agents
// @Description Returns published marketplace agents with optional filters. Cached in Redis.
// @Tags marketplace
// @Produce json
// @Param search query string false "Search by name"
// @Param tags query string false "Filter by tag (comma-separated)"
// @Param featured query bool false "Filter featured agents"
// @Param popular query bool false "Filter popular agents"
// @Param verified query bool false "Filter verified agents"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[marketplaceAgentResponse]
// @Router /v1/marketplace/agents [get]
func (h *MarketplaceHandler) List(w http.ResponseWriter, r *http.Request) {
	cacheKey := h.listCacheKey(r)

	if cached, err := h.redis.Get(r.Context(), cacheKey).Bytes(); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Preload("Publisher").Where("status = ?", "published")

	if search := r.URL.Query().Get("search"); search != "" {
		q = q.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if tags := r.URL.Query().Get("tags"); tags != "" {
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				q = q.Where("? = ANY(tags)", tag)
			}
		}
	}
	if r.URL.Query().Get("featured") == "true" {
		q = q.Where("featured = true")
	}
	if r.URL.Query().Get("popular") == "true" {
		q = q.Where("popular = true")
	}
	if r.URL.Query().Get("verified") == "true" {
		q = q.Where("verified_at IS NOT NULL")
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

	body, _ := json.Marshal(result)
	h.redis.Set(r.Context(), cacheKey, body, marketplaceCacheTTL)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

// GetBySlug handles GET /v1/marketplace/agents/{slug}.
// @Summary Get a marketplace agent by slug
// @Description Returns a single published marketplace agent by its URL slug. Cached in Redis.
// @Tags marketplace
// @Produce json
// @Param slug path string true "Agent slug"
// @Success 200 {object} marketplaceAgentResponse
// @Failure 404 {object} errorResponse
// @Router /v1/marketplace/agents/{slug} [get]
func (h *MarketplaceHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	cacheKey := marketplaceCachePrefix + "slug:" + slug

	if cached, err := h.redis.Get(r.Context(), cacheKey).Bytes(); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	var agent model.MarketplaceAgent
	if err := h.db.Preload("Publisher").Where("slug = ? AND status = ?", slug, "published").First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get agent"})
		return
	}

	body, _ := json.Marshal(toMarketplaceAgentResponse(agent))
	h.redis.Set(r.Context(), cacheKey, body, marketplaceCacheTTL)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

// ---------------------------------------------------------------------------
// Authenticated endpoints (JWT + org context)
// ---------------------------------------------------------------------------

// Create handles POST /v1/marketplace/agents.
// @Summary Publish agent to marketplace
// @Description Copies an org agent into the marketplace as a draft listing.
// @Tags marketplace
// @Accept json
// @Produce json
// @Param body body createMarketplaceAgentRequest true "Agent to publish"
// @Success 201 {object} marketplaceAgentResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/marketplace/agents [post]
func (h *MarketplaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	var req createMarketplaceAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND status = ?", req.AgentID, org.ID, "active").First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return
	}

	requiredIntegrations := extractRequiredIntegrations(agent.Integrations)

	ma := model.MarketplaceAgent{
		OrgID:                org.ID,
		PublisherID:          user.ID,
		SourceAgentID:        &agent.ID,
		Name:                 agent.Name,
		Description:          agent.Description,
		SystemPrompt:         agent.SystemPrompt,
		Instructions:         agent.Instructions,
		Model:                agent.Model,
		SandboxType:          agent.SandboxType,
		Tools:                agent.Tools,
		McpServers:           agent.McpServers,
		Skills:               agent.Skills,
		Integrations:         agent.Integrations,
		AgentConfig:          agent.AgentConfig,
		Permissions:          agent.Permissions,
		Team:                 agent.Team,
		SharedMemory:         agent.SharedMemory,
		RequiredIntegrations: requiredIntegrations,
		Slug:                 model.GenerateSlug(agent.Name),
		Status:               "draft",
	}

	if err := h.db.Create(&ma).Error; err != nil {
		slog.Error("failed to create marketplace agent", "error", err, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create marketplace listing"})
		return
	}

	ma.Publisher = *user
	slog.Info("marketplace agent created", "marketplace_id", ma.ID, "agent_id", agent.ID, "publisher_id", user.ID)
	writeJSON(w, http.StatusCreated, toMarketplaceAgentResponse(ma))
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

// ---------------------------------------------------------------------------
// Admin endpoints
// ---------------------------------------------------------------------------

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

type adminUpdateMarketplaceAgentRequest struct {
	Featured *bool   `json:"featured,omitempty"`
	Popular  *bool   `json:"popular,omitempty"`
	Verified *bool   `json:"verified,omitempty"`
	Flagged  *bool   `json:"flagged,omitempty"`
	Status   *string `json:"status,omitempty"`
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *MarketplaceHandler) listCacheKey(r *http.Request) string {
	params := r.URL.Query()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params.Get(k))
	}

	hash := sha256.Sum256([]byte(strings.Join(parts, "&")))
	return fmt.Sprintf("%slist:%x", marketplaceCachePrefix, hash[:8])
}

func extractRequiredIntegrations(integrations model.JSON) []string {
	if len(integrations) == 0 {
		return []string{}
	}

	var result []string
	for key := range integrations {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
