package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// maxOrgsOwnedPerUser caps how many organizations a single user may own.
// Enforced by OrgHandler.Create to prevent resource-exhaustion abuse (see #62).
const maxOrgsOwnedPerUser = 10

type OrgHandler struct {
	db *gorm.DB
}

func NewOrgHandler(db *gorm.DB) *OrgHandler {
	return &OrgHandler{db: db}
}

type createOrgRequest struct {
	Name string `json:"name"`
}

type orgResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RateLimit int    `json:"rate_limit"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

// Create handles POST /v1/orgs.
// @Summary Create an organization
// @Description Creates a new organization and adds the requesting user as an admin member.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body createOrgRequest true "Organization name"
// @Success 201 {object} orgResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs [post]
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	// Per-user org creation quota (see #62).
	userID, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	var ownedCount int64
	if err := h.db.Model(&model.OrgMembership{}).
		Where("user_id = ? AND role = ?", userID, "owner").
		Count(&ownedCount).Error; err != nil {
		slog.Error("failed to count owned orgs", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}
	if ownedCount >= int64(maxOrgsOwnedPerUser) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization creation limit reached"})
		return
	}

	var org model.Org
	var membership model.OrgMembership

	err := h.db.Transaction(func(tx *gorm.DB) error {
		org = model.Org{
			Name: req.Name,
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership = model.OrgMembership{
			UserID: userID,
			OrgID:  org.ID,
			Role:   "owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		return nil
	})
	if err != nil {
		slog.Error("failed to create org", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}

	writeJSON(w, http.StatusCreated, orgResponse{
		ID:        org.ID.String(),
		Name:      org.Name,
		RateLimit: org.RateLimit,
		Active:    org.Active,
		CreatedAt: org.CreatedAt.Format(time.RFC3339),
	})
}

// Current handles GET /v1/orgs/current.
// @Summary Get current organization
// @Description Returns the organization resolved from the request's auth context.
// @Tags orgs
// @Produce json
// @Success 200 {object} orgResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [get]
func (h *OrgHandler) Current(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	writeJSON(w, http.StatusOK, orgResponse{
		ID:        org.ID.String(),
		Name:      org.Name,
		RateLimit: org.RateLimit,
		Active:    org.Active,
		CreatedAt: org.CreatedAt.Format(time.RFC3339),
	})
}
