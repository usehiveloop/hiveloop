package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary List employee agent templates
// @Description Returns category-scoped employee subagent templates and whether each one is already installed on the employee.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 200 {array} employeeAgentTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/agent-templates [get]
func (h *EmployeeHandler) ListAgentTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	employee, ok := h.loadEmployeeForTemplateRequest(w, r, org.ID)
	if !ok {
		return
	}

	installed, err := loadEmployeeTemplateSubagents(ctx, h.db, employee.ID)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "load employee template subagents",
			"error", err, "agent_id", employee.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee templates"})
		return
	}

	writeJSON(w, http.StatusOK, employeeAgentTemplateResponses(employeeCategory(employee), installed))
}

// @Summary Install an employee agent template
// @Description Idempotently adds or updates a template subagent on the employee, attaches default subagent skills, and syncs runtime config.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param slug path string true "Template slug"
// @Success 200 {object} installEmployeeAgentTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/agent-templates/{slug}/install [post]
func (h *EmployeeHandler) InstallAgentTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	employee, ok := h.loadEmployeeForTemplateRequest(w, r, org.ID)
	if !ok {
		return
	}

	if upgrade, ok, err := activeEmployeeSandboxUpgrade(ctx, h.db, org.ID, employee.ID); err != nil {
		log.ErrorContext(ctx, "load active employee sandbox upgrade", "error", err, "agent_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load active upgrade"})
		return
	} else if ok {
		writeEmployeeUpgradeConflict(w, upgrade)
		return
	}

	template := employeeAgentTemplateBySlug(chi.URLParam(r, "slug"))
	if template == nil || template.Category != employeeCategory(employee) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent template not found"})
		return
	}

	hasProfile, err := h.employeeHasActiveSlackProfile(ctx, org.ID, employee.ID)
	if err != nil {
		log.ErrorContext(ctx, "count employee profiles", "error", err, "agent_id", employee.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee profiles"})
		return
	}
	if !hasProfile {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee must have an active slack profile"})
		return
	}

	team, err := h.ensureEmployeeTeam(ctx, employee)
	if err != nil {
		log.ErrorContext(ctx, "ensure employee team for template install", "error", err, "agent_id", employee.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set up employee team"})
		return
	}

	var subagent *model.Agent
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created, err := template.EnsureTx(ctx, h, tx, employee, team)
		if err != nil {
			return err
		}
		subagent = created
		return nil
	})
	if err != nil {
		log.ErrorContext(ctx, "install employee agent template",
			"error", err, "agent_id", employee.ID, "template_slug", template.Slug, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to install agent template"})
		return
	}
	h.attachGlobalSkills(ctx, subagent.ID, template.DefaultSkillNames)

	sb, err := h.ensureEmployeeSandbox(ctx, employee)
	if err != nil {
		log.ErrorContext(ctx, "provision employee sandbox during template install", "error", err, "agent_id", employee.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to provision employee sandbox"})
		return
	}
	syncResp, err := h.runEmployeeSync(ctx, employee, sb)
	if err != nil {
		log.ErrorContext(ctx, "sync employee config after template install",
			"error", err, "agent_id", employee.ID, "sandbox_id", sb.ID, "template_slug", template.Slug)
		logging.Capture(ctx, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sandbox rejected sync"})
		return
	}

	subSummary := employeeSubagentSummaryFromAgent(*subagent)
	installed := map[string]*model.Agent{template.Slug: subagent}
	writeJSON(w, http.StatusOK, installEmployeeAgentTemplateResponse{
		Template: template.toResponse(installed),
		Subagent: subSummary,
		Sync:     toSyncResponseDTO(syncResp),
	})
}

func (h *EmployeeHandler) loadEmployeeForTemplateRequest(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (*model.Agent, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return nil, false
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND is_employee = true", agentID, orgID).
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load employee for template request",
			"error", err, "agent_id", agentID, "org_id", orgID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil, false
	}
	return &agent, true
}
