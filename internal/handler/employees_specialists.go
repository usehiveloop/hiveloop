package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type employeeSpecialistResponse struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	AgentType   string `json:"agent_type"`
	Version     int    `json:"version"`
	Enabled     bool   `json:"enabled"`
}

// ListSpecialists handles GET /v1/employees/{id}/specialists.
// @Summary List employee specialists
// @Description Returns all code-defined specialists and whether Hivy currently has each enabled.
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Success 200 {array} employeeSpecialistResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/specialists [get]
func (h *EmployeeHandler) ListSpecialists(w http.ResponseWriter, r *http.Request) {
	employee, ok := h.loadEmployeeFromRequest(w, r)
	if !ok {
		return
	}
	disabled := disabledSpecialistSet(employee.DisabledSpecialists)
	out := make([]employeeSpecialistResponse, 0, len(employeeAgentTemplates))
	for i := range employeeAgentTemplates {
		t := employeeAgentTemplates[i]
		out = append(out, employeeSpecialistResponse{
			Slug:        t.Slug,
			Name:        t.Name,
			Description: t.Description,
			AgentType:   t.AgentType,
			Version:     t.Version,
			Enabled:     !disabled[t.Slug],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// EnableSpecialist handles POST /v1/employees/{id}/specialists/{slug}.
// @Summary Enable an employee specialist
// @Description Removes a specialist slug from Hivy's disabled specialist list.
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Param slug path string true "Specialist slug"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/specialists/{slug} [post]
func (h *EmployeeHandler) EnableSpecialist(w http.ResponseWriter, r *http.Request) {
	h.setSpecialistDisabled(w, r, false)
}

// DisableSpecialist handles DELETE /v1/employees/{id}/specialists/{slug}.
// @Summary Disable an employee specialist
// @Description Adds a specialist slug to Hivy's disabled specialist list.
// @Tags employees
// @Produce json
// @Param id path string true "Employee ID"
// @Param slug path string true "Specialist slug"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/specialists/{slug} [delete]
func (h *EmployeeHandler) DisableSpecialist(w http.ResponseWriter, r *http.Request) {
	h.setSpecialistDisabled(w, r, true)
}

func (h *EmployeeHandler) setSpecialistDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	employee, ok := h.loadEmployeeFromRequest(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if employeeAgentTemplateBySlug(slug) == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not found"})
		return
	}
	next := setDisabledSpecialist(employee.DisabledSpecialists, slug, disabled)
	if err := h.db.WithContext(r.Context()).
		Model(&model.Agent{}).
		Where("id = ?", employee.ID).
		Update("disabled_specialists", pq.StringArray(next)).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update specialist"})
		return
	}
	employee.DisabledSpecialists = next
	status := "enabled"
	if disabled {
		status = "disabled"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (h *EmployeeHandler) loadEmployeeFromRequest(w http.ResponseWriter, r *http.Request) (*model.Agent, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return nil, false
	}
	var employee model.Agent
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", id, org.ID, "archived").
		First(&employee).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil, false
	}
	return &employee, true
}

func disabledSpecialistSet(slugs []string) map[string]bool {
	out := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		out[slug] = true
	}
	return out
}

func setDisabledSpecialist(slugs []string, slug string, disabled bool) []string {
	seen := disabledSpecialistSet(slugs)
	if disabled {
		seen[slug] = true
	} else {
		delete(seen, slug)
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	return out
}
