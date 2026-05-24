package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type employeeSpecialistResponse struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	SpecialistType string `json:"specialist_type"`
	Version        int    `json:"version"`
	Attached       bool   `json:"attached"`
}

// ListSpecialists handles GET /v1/employees/{id}/specialists.
// @Summary List employee specialists
// @Description Returns all global specialists and whether Hivy currently has each attached.
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
	attached := attachedSpecialistSet(employee.AttachedSpecialists)
	defs := h.specialists.List()
	out := make([]employeeSpecialistResponse, 0, len(defs))
	for _, def := range defs {
		out = append(out, employeeSpecialistResponse{
			Slug:           def.Slug,
			Name:           def.Name,
			Description:    def.Description,
			SpecialistType: def.SpecialistType,
			Version:        def.Version,
			Attached:       attached[def.Slug],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// AttachSpecialist handles POST /v1/employees/{id}/specialists/{slug}.
// @Summary Attach an employee specialist
// @Description Adds a global specialist slug to Hivy's attached specialist list.
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
	h.setSpecialistAttached(w, r, true)
}

// DetachSpecialist handles DELETE /v1/employees/{id}/specialists/{slug}.
// @Summary Detach an employee specialist
// @Description Removes a global specialist slug from Hivy's attached specialist list.
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
	h.setSpecialistAttached(w, r, false)
}

func (h *EmployeeHandler) setSpecialistAttached(w http.ResponseWriter, r *http.Request, attached bool) {
	employee, ok := h.loadEmployeeFromRequest(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if _, ok := h.specialists.BySlug(slug); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not found"})
		return
	}
	next := setAttachedSpecialist(employee.AttachedSpecialists, slug, attached)
	if err := h.db.WithContext(r.Context()).
		Model(&model.Employee{}).
		Where("id = ?", employee.ID).
		Update("attached_specialists", pq.StringArray(next)).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update specialist"})
		return
	}
	employee.AttachedSpecialists = next
	status := "attached"
	if !attached {
		status = "detached"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (h *EmployeeHandler) loadEmployeeFromRequest(w http.ResponseWriter, r *http.Request) (*model.Employee, bool) {
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
	var employee model.Employee
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

func attachedSpecialistSet(slugs []string) map[string]bool {
	out := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		out[slug] = true
	}
	return out
}

func setAttachedSpecialist(slugs []string, slug string, attached bool) []string {
	seen := attachedSpecialistSet(slugs)
	if attached {
		seen[slug] = true
	} else {
		delete(seen, slug)
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
