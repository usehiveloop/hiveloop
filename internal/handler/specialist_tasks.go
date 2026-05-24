package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bridge"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/specialists"
)

// SpecialistBridgeClient is the subset of bridge runtime operations used by
// specialist coordination. Keeping this seam small makes the bridge contract
// testable without provisioning a real sandbox.
type SpecialistBridgeClient interface {
	CreateConversation(ctx context.Context, agentID string) (*bridge.CreateConversationResponse, error)
	SendMessage(ctx context.Context, convID string, content string) error
	EndConversation(ctx context.Context, convID string) error
}

type SpecialistTaskHandlerHooks struct {
	CreateSpecialistSandbox func(ctx context.Context, agent *model.Employee, extraEnv map[string]string) (*model.Sandbox, error)
	PushSpecialistToSandbox func(ctx context.Context, agent *model.Employee, sb *model.Sandbox) error
	GetBridgeClient         func(ctx context.Context, sb *model.Sandbox) (SpecialistBridgeClient, error)
	StopSandbox             func(ctx context.Context, sb *model.Sandbox) error
	DeleteSandbox           func(ctx context.Context, sb *model.Sandbox) error
	TaskDriveUploadURL      func(employeeID uuid.UUID, taskID uuid.UUID) string
	EmployeeCallbackRuntime employeeCallbackSandboxSpecialists
}

type SpecialistTaskHandler struct {
	db      *gorm.DB
	encKey  *crypto.SymmetricKey
	hooks   SpecialistTaskHandlerHooks
	catalog *specialists.Catalog
}

func NewSpecialistTaskHandler(db *gorm.DB, encKey *crypto.SymmetricKey, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher, catalog ...*specialists.Catalog) *SpecialistTaskHandler {
	hooks := SpecialistTaskHandlerHooks{}
	if orchestrator != nil {
		hooks.CreateSpecialistSandbox = orchestrator.CreateSpecialistSandboxWithEnv
		hooks.GetBridgeClient = func(ctx context.Context, sb *model.Sandbox) (SpecialistBridgeClient, error) {
			return orchestrator.GetBridgeClient(ctx, sb)
		}
		hooks.StopSandbox = orchestrator.StopSandbox
		hooks.DeleteSandbox = orchestrator.DeleteSandboxResource
		hooks.TaskDriveUploadURL = orchestrator.EmployeeTaskDriveUploadURL
		hooks.EmployeeCallbackRuntime = orchestrator
	}
	if pusher != nil {
		hooks.PushSpecialistToSandbox = pusher.PushSpecialistToSandbox
	}
	return NewSpecialistTaskHandlerWithHooks(db, encKey, hooks, catalog...)
}

func NewSpecialistTaskHandlerWithHooks(db *gorm.DB, encKey *crypto.SymmetricKey, hooks SpecialistTaskHandlerHooks, catalog ...*specialists.Catalog) *SpecialistTaskHandler {
	return &SpecialistTaskHandler{db: db, encKey: encKey, hooks: hooks, catalog: specialistCatalogFromArgs(catalog...)}
}

// authEmployee verifies the bridge bearer token for the employee in the URL.
// On failure it writes the error response and returns nil — callers must return.
func (h *SpecialistTaskHandler) authEmployee(w http.ResponseWriter, r *http.Request) *model.Employee {
	if h.encKey == nil {
		captureSpecialistFailure(r.Context(), "auth", errors.New("encryption key is not configured"), specialistSentryContext{
			Operation: "configuration",
		})
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "specialist endpoints not configured"})
		return nil
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return nil
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil
	}

	var agent model.Employee
	if err := h.db.Where("id = ? AND is_employee = true", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil
		}
		captureSpecialistFailure(r.Context(), "auth", err, specialistSentryContext{
			Operation:  "load_employee",
			EmployeeID: agentID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil
	}

	var sb model.Sandbox
	if err := h.db.
		Where("employee_id = ? AND status NOT IN (?, ?)", agentID, "archived", "error").
		Order("created_at DESC").
		First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found for employee"})
			return nil
		}
		captureSpecialistFailure(r.Context(), "auth", err, specialistSentryContext{
			Operation:  "load_employee_sandbox",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil
	}

	wantKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "employee_id", agentID, "error", err)
		captureSpecialistFailure(r.Context(), "auth", err, specialistSentryContext{
			Operation:  "decrypt_bridge_key",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
			SandboxID:  sb.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		captureSpecialistWarning(r.Context(), "auth", errors.New("invalid bridge api key"), specialistSentryContext{
			Operation:  "invalid_bridge_key",
			EmployeeID: agentID,
			OrgID:      uuidValue(agent.OrgID),
			SandboxID:  sb.ID,
		})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
		return nil
	}

	return &agent
}

func specialistID(slug string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("hivy-specialist:"+slug))
}

func specialistAgentFromDefinition(employee *model.Employee, def specialists.Definition) model.Employee {
	description := def.Description
	orgID := uuidValue(employee.OrgID)
	return model.Employee{
		ID:            specialistID(def.Slug),
		OrgID:         &orgID,
		Name:          def.Name,
		Description:   &description,
		SystemPrompt:  def.SystemPrompt,
		Model:         employee.Model,
		Tools:         employee.Tools,
		McpServers:    employee.McpServers,
		Skills:        employee.Skills,
		RuntimeConfig: employee.RuntimeConfig,
		Permissions:   employee.Permissions,
		Resources:     employee.Resources,
		Harness:       employeeSpecialistHarness,
		Status:        "active",
	}
}

// validateSpecialist checks that slug refers to an attached global specialist.
func (h *SpecialistTaskHandler) validateSpecialist(ctx context.Context, w http.ResponseWriter, employee *model.Employee, slug string) (*specialists.Definition, uuid.UUID, bool) {
	def, exists := h.catalog.BySlug(slug)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not found for this employee"})
		return nil, uuid.Nil, false
	}
	if !attachedSpecialistSet(employee.AttachedSpecialists)[slug] {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "specialist not attached for this employee"})
		return nil, uuid.Nil, false
	}
	return def, specialistID(slug), true
}
