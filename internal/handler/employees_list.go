package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type employeeSpecialistSummary struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	AvatarURL              *string `json:"avatar_url,omitempty"`
	Description            *string `json:"description,omitempty"`
	Status                 string  `json:"status"`
	TemplateSlug           *string `json:"template_slug,omitempty"`
	TemplateSpecialistType *string `json:"template_specialist_type,omitempty"`
	DefaultModel           string  `json:"default_model"`
	ConfiguredModel        *string `json:"configured_model,omitempty"`
	EffectiveModel         string  `json:"effective_model"`
}

type employeeSandboxSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	ExternalID   string  `json:"external_id"`
	ErrorMessage *string `json:"error_message,omitempty"`
	LastActiveAt *string `json:"last_active_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	snapshotID   *string
}

type employeeListItem struct {
	employeeResponse
	UpgradeAvailable bool                        `json:"upgrade_available"`
	Specialists      []employeeSpecialistSummary `json:"specialists"`
	Sandbox          *employeeSandboxSummary     `json:"sandbox,omitempty"`
}

// @Summary List AI employees
// @Description Returns all employees in the org with enabled specialists,
// @Description skills (metadata only — no bundle content),
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param status query string false "Filter by status (draft, active, archived)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[employeeListItem]
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees [get]
func (h *EmployeeHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.WithContext(r.Context()).
		Preload("Credential").
		Where("employees.org_id = ?", org.ID)

	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("employees.status = ?", status)
	}

	q = applyPagination(q, cursor, limit)

	var agents []model.Employee
	if err := q.Find(&agents).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list employees"})
		return
	}

	hasMore := len(agents) > limit
	if hasMore {
		agents = agents[:limit]
	}

	agentIDs := make([]uuid.UUID, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}

	triggers := h.loadEmployeeTriggers(agentIDs...)
	skills := h.loadEmployeeSkills(agentIDs...)
	sandboxes := loadLatestSandboxPerAgent(h.db, org.ID, agentIDs)
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	items := make([]employeeListItem, len(agents))
	for i, a := range agents {
		base := toEmployeeResponse(a)
		base.Triggers = triggers[a.ID]
		base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &a, skills[a.ID])
		subs := h.employeeAttachedSpecialistSummaries(a)
		base.SpecialistIDs = make([]string, len(subs))
		for j, s := range subs {
			base.SpecialistIDs[j] = s.ID
		}
		items[i] = employeeListItem{
			employeeResponse: base,
			UpgradeAvailable: employeeSandboxUpgradeAvailable(sandboxes[a.ID], currentSnapshotID),
			Specialists:      subs,
			Sandbox:          sandboxes[a.ID],
		}
	}

	result := paginatedResponse[employeeListItem]{Data: items, HasMore: hasMore}
	if hasMore {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/employees/{id}.
// @Summary Get an AI employee
// @Description Returns one employee in the org with enabled specialists,
// @Description skills (metadata only — no bundle content),
// @Description triggers, and the latest sandbox row.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 200 {object} employeeListItem
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id} [get]
func (h *EmployeeHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}

	var agent model.Employee
	if err := h.db.WithContext(r.Context()).
		Preload("Credential").
		Where("employees.id = ? AND employees.org_id = ? AND employees.status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return
	}

	base := toEmployeeResponse(agent)
	base.Triggers = h.loadEmployeeTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(r.Context(), org.ID, &agent, h.loadEmployeeSkills(agent.ID)[agent.ID])
	specialists := h.employeeAttachedSpecialistSummaries(agent)
	base.SpecialistIDs = make([]string, len(specialists))
	for i, specialist := range specialists {
		base.SpecialistIDs[i] = specialist.ID
	}
	sandbox := loadLatestSandboxPerAgent(h.db, org.ID, []uuid.UUID{agent.ID})[agent.ID]
	currentSnapshotID := h.currentEmployeeSandboxSnapshotID()

	writeJSON(w, http.StatusOK, employeeListItem{
		employeeResponse: base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, currentSnapshotID),
		Specialists:      specialists,
		Sandbox:          sandbox,
	})
}

func (h *EmployeeHandler) currentEmployeeSandboxSnapshotID() string {
	if h == nil || h.compileDeps.Cfg == nil {
		return ""
	}
	return h.compileDeps.Cfg.SandboxesRuntimeBaseImagePrefix
}

func (h *EmployeeHandler) employeeListItem(ctx context.Context, orgID uuid.UUID, agent model.Employee) employeeListItem {
	base := toEmployeeResponse(agent)
	base.Triggers = h.loadEmployeeTriggers(agent.ID)[agent.ID]
	base.AttachedSkills = h.markEmployeeSkillLocks(ctx, orgID, &agent, h.loadEmployeeSkills(agent.ID)[agent.ID])
	specialists := h.employeeAttachedSpecialistSummaries(agent)
	base.SpecialistIDs = make([]string, len(specialists))
	for i, specialist := range specialists {
		base.SpecialistIDs[i] = specialist.ID
	}
	sandbox := loadLatestSandboxPerAgent(h.db, orgID, []uuid.UUID{agent.ID})[agent.ID]
	return employeeListItem{
		employeeResponse: base,
		UpgradeAvailable: employeeSandboxUpgradeAvailable(sandbox, h.currentEmployeeSandboxSnapshotID()),
		Specialists:      specialists,
		Sandbox:          sandbox,
	}
}

func (h *EmployeeHandler) employeeAttachedSpecialistSummaries(employee model.Employee) []employeeSpecialistSummary {
	attached := attachedSpecialistSet(employee.AttachedSpecialists)
	defs := h.specialists.List()
	out := make([]employeeSpecialistSummary, 0, len(defs))
	for _, def := range defs {
		if !attached[def.Slug] {
			continue
		}
		slug := def.Slug
		specialistType := def.SpecialistType
		desc := def.Description
		configuredModel, effectiveModel := h.employeeSpecialistModels(employee, def.Slug, def.DefaultModel)
		out = append(out, employeeSpecialistSummary{
			ID:                     def.Slug,
			Name:                   def.Name,
			Description:            &desc,
			Status:                 "active",
			TemplateSlug:           &slug,
			TemplateSpecialistType: &specialistType,
			DefaultModel:           def.DefaultModel,
			ConfiguredModel:        configuredModel,
			EffectiveModel:         effectiveModel,
		})
	}
	return out
}

func employeeSandboxUpgradeAvailable(summary *employeeSandboxSummary, currentSnapshotID string) bool {
	if summary == nil {
		return false
	}
	if summary.snapshotID == nil || *summary.snapshotID == "" {
		return currentSnapshotID != ""
	}
	return *summary.snapshotID != currentSnapshotID
}
