package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	slackprov "github.com/usehiveloop/hiveloop/internal/profiles/slack"
)

type completeOnboardingRequest struct {
	Name        string `json:"name"`
	Website     string `json:"website"`
	LogoURL     string `json:"logo_url"`
	Description string `json:"description"`
}

type completeOnboardingResponse struct {
	AgentID string               `json:"agent_id"`
	Sync    syncEmployeeResponse `json:"sync"`
}

// @Summary Complete onboarding
// @Description Saves the org's business info, validates the org has at least
// @Description one employee with an active Slack profile, then runs
// @Description the same compile + sandbox-sync as POST /v1/employees/{id}/sync
// @Description on the org's first employee. On success Org.onboarded is set.
// @Tags onboarding
// @Accept json
// @Produce json
// @Param body body completeOnboardingRequest true "Business info"
// @Success 200 {object} completeOnboardingResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/onboarding/complete [post]
func (h *EmployeeHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req completeOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Website = strings.TrimSpace(req.Website)
	req.LogoURL = strings.TrimSpace(req.LogoURL)
	req.Description = strings.TrimSpace(req.Description)

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.Website != "" {
		u, err := url.Parse(req.Website)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "website must be an absolute http(s) URL"})
			return
		}
	}
	if req.LogoURL != "" {
		u, err := url.Parse(req.LogoURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "logo_url must be an absolute http(s) URL"})
			return
		}
	}

	var agent model.Agent
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND is_employee = true AND is_system = false AND deleted_at IS NULL", org.ID).
		Order("created_at ASC").Limit(1).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no employee configured for this org"})
			return
		}
		log.ErrorContext(ctx, "load first employee", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}

	var profileCount int64
	err = h.db.WithContext(ctx).Model(&model.AgentProfile{}).
		Where("agent_id = ? AND status = ? AND deleted_at IS NULL AND provider = ?",
			agent.ID, "active", slackprov.Provider).
		Count(&profileCount).Error
	if err != nil {
		log.ErrorContext(ctx, "count employee profiles", "error", err, "agent_id", agent.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee profiles"})
		return
	}
	if profileCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee must have an active slack profile"})
		return
	}

	var sb model.Sandbox
	err = h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ?", agent.ID, org.ID).
		Order("created_at DESC").Limit(1).First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			created, createErr := h.ensureEmployeeSandbox(ctx, &agent)
			if createErr != nil {
				log.ErrorContext(ctx, "provision employee sandbox during onboarding", "error", createErr,
					"agent_id", agent.ID, "org_id", org.ID)
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision employee sandbox"})
				return
			}
			sb = *created
		} else {
			log.ErrorContext(ctx, "load employee sandbox", "error", err, "agent_id", agent.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee sandbox"})
			return
		}
	}

	if err := h.db.WithContext(ctx).Model(&model.Org{}).Where("id = ?", org.ID).Updates(map[string]any{
		"name":        req.Name,
		"website":     req.Website,
		"logo_url":    req.LogoURL,
		"description": req.Description,
	}).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "another organization with this name already exists"})
			return
		}
		log.ErrorContext(ctx, "save org info", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save business info"})
		return
	}

	resp, err := h.runEmployeeSync(ctx, &agent, &sb)
	if err != nil {
		log.ErrorContext(ctx, "complete onboarding sync", "error", err,
			"agent_id", agent.ID, "sandbox_id", sb.ID, "org_id", org.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}

	if err := h.db.WithContext(ctx).Model(&model.Org{}).Where("id = ?", org.ID).
		Update("onboarded", true).Error; err != nil {
		log.ErrorContext(ctx, "set org onboarded", "error", err, "org_id", org.ID)
	}

	writeJSON(w, http.StatusOK, completeOnboardingResponse{
		AgentID: agent.ID.String(),
		Sync:    toSyncResponseDTO(resp),
	})
}
