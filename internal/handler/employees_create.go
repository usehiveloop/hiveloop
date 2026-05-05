package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Create handles POST /v1/employees.
//
// @Summary Create an AI employee
// @Description Persists an Agent (is_employee=true) and provisions a Hermes sandbox.
// @Description On any provisioning failure, the Agent is rolled back so the
// @Description endpoint is transactional from the caller's POV.
// @Tags employees
// @Accept json
// @Produce json
// @Param body body createEmployeeRequest true "Employee definition"
// @Success 201 {object} createEmployeeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees [post]
func (h *EmployeeHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)
	req.Description = strings.TrimSpace(req.Description)

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.Description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description is required"})
		return
	}
	if req.Category == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category is required"})
		return
	}
	if !isValidAgentCategory(req.Category) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid category %q", req.Category)})
		return
	}
	if req.Category != employeeCategoryEngineering {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("category %q is not yet supported for employees", req.Category)})
		return
	}
	if req.AvatarURL != "" {
		u, err := url.Parse(req.AvatarURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url must be an absolute http(s) URL"})
			return
		}
	}

	choice, err := pickEmployeeCredential(h.db)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "pick employee credential", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no provider credential available for new employees"})
		return
	}

	desc := req.Description
	cat := req.Category
	agent := model.Agent{
		OrgID:        &org.ID,
		Name:         req.Name,
		Description:  &desc,
		Category:     &cat,
		SystemPrompt: engineeringSystemPrompt,
		Model:        choice.model,
		CredentialID: &choice.cred.ID,
		Harness:      employeeHarness,
		IsEmployee:   true,
		Status:       "active",
		Tools:        model.JSON{},
		McpServers:   model.JSON{},
		Skills:       model.JSON{},
		Integrations: model.JSON{},
		Resources:    model.JSON{},
		AgentConfig:  model.JSON{},
		Permissions:  model.JSON{},
	}
	if req.AvatarURL != "" {
		avatar := req.AvatarURL
		agent.AvatarURL = &avatar
	}

	if err := h.db.Create(&agent).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("employee with name %q already exists", req.Name)})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create employee agent", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create employee"})
		return
	}

	sb, err := h.orchestrator.CreateHermesSandbox(r.Context(), &agent)
	if err != nil {
		h.rollbackEmployee(r.Context(), org.ID, agent.ID)
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "provision hermes sandbox",
			"error", err, "agent_id", agent.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to provision sandbox"})
		return
	}

	writeJSON(w, http.StatusCreated, createEmployeeResponse{
		AgentID:   agent.ID.String(),
		SandboxID: sb.ID.String(),
		Status:    sb.Status,
	})
}

func (h *EmployeeHandler) rollbackEmployee(ctx context.Context, orgID, agentID uuid.UUID) {
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND agent_id = ?", orgID, agentID).Delete(&model.Sandbox{}).Error; err != nil {
			return fmt.Errorf("delete sandbox: %w", err)
		}
		if err := tx.Where("org_id = ? AND id = ?", orgID, agentID).Delete(&model.Agent{}).Error; err != nil {
			return fmt.Errorf("delete agent: %w", err)
		}
		return nil
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "rollback employee", "error", err,
			"agent_id", agentID, "org_id", orgID)
	}
}
