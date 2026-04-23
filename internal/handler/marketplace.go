package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
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

func NewMarketplaceHandler(db *gorm.DB, redisClient *redis.Client) *MarketplaceHandler {
	return &MarketplaceHandler{db: db, redis: redisClient}
}

type createMarketplaceAgentRequest struct {
	AgentID string `json:"agent_id"`
}

// marketplacePublicAgentResponse is the sanitized "discovery card" shape
// returned to unauthenticated callers. It intentionally excludes fields that
// disclose internal agent configuration — system prompts, tools, MCP servers,
// agent config, permissions, instructions, provider prompts, and source
// agent linkage. Authenticated marketplace consumers (e.g. install flows)
// continue to receive the full marketplaceAgentResponse shape.
type marketplacePublicAgentResponse struct {
	ID                   string   `json:"id"`
	Slug                 string   `json:"slug"`
	Name                 string   `json:"name"`
	Description          *string  `json:"description,omitempty"`
	Avatar               *string  `json:"avatar,omitempty"`
	Model                string   `json:"model"`
	Team                 string   `json:"team"`
	RequiredIntegrations []string `json:"required_integrations"`
	Tags                 []string `json:"tags"`
	Status               string   `json:"status"`
	Featured             bool     `json:"featured"`
	Popular              bool     `json:"popular"`
	Verified             bool     `json:"verified"`
	InstallCount         int      `json:"install_count"`
	PublisherName        string   `json:"publisher_name,omitempty"`
	PublishedAt          *string  `json:"published_at,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

func toMarketplacePublicAgentResponse(ma model.MarketplaceAgent) marketplacePublicAgentResponse {
	resp := marketplacePublicAgentResponse{
		ID:                   ma.ID.String(),
		Slug:                 ma.Slug,
		Name:                 ma.Name,
		Description:          ma.Description,
		Avatar:               ma.Avatar,
		Model:                ma.Model,
		Team:                 ma.Team,
		RequiredIntegrations: ma.RequiredIntegrations,
		Tags:                 ma.Tags,
		Status:               ma.Status,
		Featured:             ma.Featured,
		Popular:              ma.Popular,
		Verified:             ma.VerifiedAt != nil,
		InstallCount:         ma.InstallCount,
		PublisherName:        ma.Publisher.Name,
		CreatedAt:            ma.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            ma.UpdatedAt.Format(time.RFC3339),
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
