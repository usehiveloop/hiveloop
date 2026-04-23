package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type marketplaceAgentResponse struct {
	ID                   string     `json:"id"`
	Slug                 string     `json:"slug"`
	Name                 string     `json:"name"`
	Description          *string    `json:"description,omitempty"`
	Avatar               *string    `json:"avatar,omitempty"`
	SystemPrompt         string     `json:"system_prompt"`
	Instructions         *string    `json:"instructions,omitempty"`
	Model                string     `json:"model"`
	SandboxType          string     `json:"sandbox_type"`
	Tools                model.JSON `json:"tools"`
	McpServers           model.JSON `json:"mcp_servers"`
	Skills               model.JSON `json:"skills"`
	Integrations         model.JSON `json:"integrations"`
	AgentConfig          model.JSON `json:"agent_config"`
	Permissions          model.JSON `json:"permissions"`
	Team                 string     `json:"team"`
	SharedMemory         bool       `json:"shared_memory"`
	RequiredIntegrations []string   `json:"required_integrations"`
	Tags                 []string   `json:"tags"`
	Status               string     `json:"status"`
	Featured             bool       `json:"featured"`
	Popular              bool       `json:"popular"`
	Verified             bool       `json:"verified"`
	Flagged              bool       `json:"flagged"`
	InstallCount         int        `json:"install_count"`
	PublisherID          string     `json:"publisher_id"`
	PublisherName        string     `json:"publisher_name,omitempty"`
	SourceAgentID        *string    `json:"source_agent_id,omitempty"`
	PublishedAt          *string    `json:"published_at,omitempty"`
	CreatedAt            string     `json:"created_at"`
	UpdatedAt            string     `json:"updated_at"`
}

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
