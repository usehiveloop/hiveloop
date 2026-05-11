package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

// jsonRoundTripToMap marshal-unmarshals into model.JSON so typed Go structs
// can land in JSONB columns (model.JSON is map[string]any).
func jsonRoundTripToMap(v any) (model.JSON, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	out := model.JSON{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return out, nil
}

type AgentProfileHandler struct {
	db    *gorm.DB
	kms   *crypto.KeyWrapper
	nango *nango.Client
}

func NewAgentProfileHandler(db *gorm.DB, kms *crypto.KeyWrapper, nangoClient *nango.Client) *AgentProfileHandler {
	return &AgentProfileHandler{db: db, kms: kms, nango: nangoClient}
}

type agentProfileResponse struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	AgentID        string     `json:"agent_id"`
	Provider       string     `json:"provider"`
	ExternalID     string     `json:"external_id"`
	Label          string     `json:"label"`
	Identity       model.JSON `json:"identity"`
	Config         model.JSON `json:"config"`
	Status         string     `json:"status"`
	StatusReason   string     `json:"status_reason,omitempty"`
	LastVerifiedAt *string    `json:"last_verified_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

func toAgentProfileResponse(p model.AgentProfile) agentProfileResponse {
	resp := agentProfileResponse{
		ID:           p.ID.String(),
		OrgID:        p.OrgID.String(),
		AgentID:      p.AgentID.String(),
		Provider:     p.Provider,
		ExternalID:   p.ExternalID,
		Label:        p.Label,
		Identity:     p.Identity,
		Config:       p.Config,
		Status:       p.Status,
		StatusReason: p.StatusReason,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
	}
	if p.LastVerifiedAt != nil {
		s := p.LastVerifiedAt.Format(time.RFC3339)
		resp.LastVerifiedAt = &s
	}
	return resp
}

var errAgentNotFound = errors.New("agent not found")

func (h *AgentProfileHandler) resolveEmployeeAgent(r *http.Request) (model.Agent, uuid.UUID, error) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		return model.Agent{}, uuid.Nil, errors.New("missing org context")
	}
	rawID := chi.URLParam(r, "agentID")
	if rawID == "" {
		rawID = chi.URLParam(r, "id")
	}
	agentID, err := uuid.Parse(rawID)
	if err != nil {
		return model.Agent{}, org.ID, errors.New("invalid agent id")
	}
	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", agentID, org.ID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Agent{}, org.ID, errAgentNotFound
		}
		return model.Agent{}, org.ID, err
	}
	if !agent.IsEmployee {
		return model.Agent{}, org.ID, errors.New("profiles can only be attached to AI employees")
	}
	return agent, org.ID, nil
}
