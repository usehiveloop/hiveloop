package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func commandsToString(cmds []string) string {
	return strings.Join(cmds, "\n")
}

func commandsToArray(cmdStr string) []string {
	if cmdStr == "" {
		return []string{}
	}
	return strings.Split(cmdStr, "\n")
}

// TemplateBuildable is the interface for building sandbox templates.
type TemplateBuildable interface {
	BuildTemplate(ctx context.Context, tmpl *model.SandboxTemplate)
	DeleteTemplate(ctx context.Context, externalID string) error
}

var _ TemplateBuildable = (*sandbox.Orchestrator)(nil)

type SandboxTemplateHandler struct {
	db       *gorm.DB
	builder  TemplateBuildable
	enqueuer enqueue.TaskEnqueuer
}

func NewSandboxTemplateHandler(db *gorm.DB, builder TemplateBuildable, enqueuer enqueue.TaskEnqueuer) *SandboxTemplateHandler {
	return &SandboxTemplateHandler{db: db, builder: builder, enqueuer: enqueuer}
}

type createSandboxTemplateRequest struct {
	Name           string     `json:"name"`
	BuildCommands  []string   `json:"build_commands"`
	Config         model.JSON `json:"config,omitempty"`
	BaseTemplateID *string    `json:"base_template_id,omitempty"`
}

type updateSandboxTemplateRequest struct {
	Name          *string    `json:"name,omitempty"`
	BuildCommands []string   `json:"build_commands,omitempty"`
	Config        model.JSON `json:"config,omitempty"`
}

type retryBuildRequest struct {
	BuildCommands []string `json:"build_commands,omitempty"`
}

type sandboxTemplateResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Tags           model.JSON `json:"tags"`
	Size           string     `json:"size"`
	IsPublic       bool       `json:"is_public"`
	BaseTemplateID *string    `json:"base_template_id,omitempty"`
	BuildCommands  []string   `json:"build_commands"`
	ExternalID     *string    `json:"external_id,omitempty"`
	BuildStatus    string     `json:"build_status"`
	BuildError     *string    `json:"build_error,omitempty"`
	BuildLogs      string     `json:"build_logs,omitempty"`
	Config         model.JSON `json:"config"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

func toSandboxTemplateResponse(t model.SandboxTemplate) sandboxTemplateResponse {
	cmds := []string{}
	if t.BuildCommands != "" {
		cmds = []string{t.BuildCommands}
	}
	resp := sandboxTemplateResponse{
		ID:            t.ID.String(),
		Name:          t.Name,
		Slug:          t.Slug,
		Tags:          t.Tags,
		Size:          t.Size,
		IsPublic:      t.OrgID == nil,
		BuildCommands: cmds,
		ExternalID:    t.ExternalID,
		BuildStatus:   t.BuildStatus,
		BuildError:    t.BuildError,
		BuildLogs:     t.BuildLogs,
		Config:        t.Config,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.Format(time.RFC3339),
	}
	if t.BaseTemplateID != nil {
		baseIDStr := t.BaseTemplateID.String()
		resp.BaseTemplateID = &baseIDStr
	}
	return resp
}

// Create handles POST /v1/sandbox-templates.
// @Summary Create a sandbox template
// @Description Creates a new sandbox template with build commands.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param body body createSandboxTemplateRequest true "Template details"
// @Success 201 {object} sandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates [post]
func (h *SandboxTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	tmpl := model.SandboxTemplate{
		OrgID:         &org.ID,
		Name:          req.Name,
		BuildCommands: commandsToString(req.BuildCommands),
		BuildStatus:   "pending",
		Config:        req.Config,
		Tags:          model.JSON{},
	}
	if tmpl.Config == nil {
		tmpl.Config = model.JSON{}
	}

	if req.BaseTemplateID != nil && *req.BaseTemplateID != "" {
		baseID, err := uuid.Parse(*req.BaseTemplateID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid base_template_id"})
			return
		}
		var baseTmpl model.SandboxTemplate
		if err := h.db.Where("id = ? AND org_id IS NULL AND build_status = ?", baseID, "ready").First(&baseTmpl).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "base template not found or not ready"})
			return
		}
		tmpl.BaseTemplateID = &baseID
		tmpl.Size = baseTmpl.Size
	}

	tmpl.Slug = fmt.Sprintf("hiveloop-tmpl-%s", uuid.New().String()[:8])

	if err := h.db.Create(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sandbox template"})
		return
	}

	writeJSON(w, http.StatusCreated, toSandboxTemplateResponse(tmpl))
}

// List handles GET /v1/sandbox-templates.
// @Summary List sandbox templates
// @Description Returns sandbox templates for the current organization.
// @Tags sandbox-templates
// @Produce json
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sandboxTemplateResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates [get]
func (h *SandboxTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Where("org_id = ?", org.ID)
	q = applyPagination(q, cursor, limit)

	var templates []model.SandboxTemplate
	if err := q.Find(&templates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandbox templates"})
		return
	}

	hasMore := len(templates) > limit
	if hasMore {
		templates = templates[:limit]
	}

	resp := make([]sandboxTemplateResponse, len(templates))
	for i, t := range templates {
		resp[i] = toSandboxTemplateResponse(t)
	}

	result := paginatedResponse[sandboxTemplateResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := templates[len(templates)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}
