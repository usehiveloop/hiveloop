package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

type employeeSpecialistResponse struct {
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	SpecialistType  string  `json:"specialist_type"`
	Version         int     `json:"version"`
	Attached        bool    `json:"attached"`
	DefaultModel    string  `json:"default_model"`
	ConfiguredModel *string `json:"configured_model,omitempty"`
	EffectiveModel  string  `json:"effective_model"`
}

type updateEmployeeSpecialistRequest struct {
	Model json.RawMessage `json:"model"`
}

var (
	errMissingSpecialistModel = errors.New("model is required")
	errInvalidSpecialistModel = errors.New("model must be a string or null")
)

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
		configuredModel, effectiveModel := h.employeeSpecialistModels(*employee, def.Slug, def.DefaultModel)
		out = append(out, employeeSpecialistResponse{
			Slug:            def.Slug,
			Name:            def.Name,
			Description:     def.Description,
			SpecialistType:  def.SpecialistType,
			Version:         def.Version,
			Attached:        attached[def.Slug],
			DefaultModel:    def.DefaultModel,
			ConfiguredModel: configuredModel,
			EffectiveModel:  effectiveModel,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// UpdateSpecialist handles PATCH /v1/employees/{id}/specialists/{slug}.
// @Summary Update an employee specialist config
// @Description Sets or clears the model override for a specialist attached to Hivy.
// @Tags employees
// @Accept json
// @Produce json
// @Param id path string true "Employee ID"
// @Param slug path string true "Specialist slug"
// @Param body body updateEmployeeSpecialistRequest true "Specialist config patch"
// @Success 200 {object} employeeSpecialistResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/specialists/{slug} [patch]
func (h *EmployeeHandler) UpdateSpecialist(w http.ResponseWriter, r *http.Request) {
	employee, ok := h.loadEmployeeFromRequest(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	def, ok := h.specialists.BySlug(slug)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not found"})
		return
	}
	modelID, err := parseSpecialistModelPatch(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if modelID != "" {
		if err := validateEmployeeModel(h.registry, modelID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	next := employeeruntime.SetSpecialistModelOverride(employee.RuntimeConfig, slug, modelID)
	if err := h.db.WithContext(r.Context()).
		Model(&model.Employee{}).
		Where("id = ?", employee.ID).
		Update("runtime_config", next).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update specialist"})
		return
	}
	employee.RuntimeConfig = next
	writeJSON(w, http.StatusOK, h.employeeSpecialistResponse(*employee, *def, attachedSpecialistSet(employee.AttachedSpecialists)[slug]))
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

func (h *EmployeeHandler) employeeSpecialistResponse(employee model.Employee, def specialists.Definition, attached bool) employeeSpecialistResponse {
	configuredModel, effectiveModel := h.employeeSpecialistModels(employee, def.Slug, def.DefaultModel)
	return employeeSpecialistResponse{
		Slug:            def.Slug,
		Name:            def.Name,
		Description:     def.Description,
		SpecialistType:  def.SpecialistType,
		Version:         def.Version,
		Attached:        attached,
		DefaultModel:    def.DefaultModel,
		ConfiguredModel: configuredModel,
		EffectiveModel:  effectiveModel,
	}
}

func (h *EmployeeHandler) employeeSpecialistModels(employee model.Employee, slug string, defaultModel string) (*string, string) {
	configured := employeeruntime.SpecialistModelOverride(employee.RuntimeConfig, slug)
	var configuredPtr *string
	if configured != "" {
		configuredPtr = &configured
	}
	effective := employeeruntime.EffectiveSpecialistModel(employee.RuntimeConfig, slug, defaultModel)
	return configuredPtr, effective
}

func parseSpecialistModelPatch(r *http.Request) (string, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return "", err
	}
	value, ok := raw["model"]
	if !ok {
		return "", errMissingSpecialistModel
	}
	if string(value) == "null" {
		return "", nil
	}
	var modelID string
	if err := json.Unmarshal(value, &modelID); err != nil {
		return "", errInvalidSpecialistModel
	}
	return strings.TrimSpace(modelID), nil
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
