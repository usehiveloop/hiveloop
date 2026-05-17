package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

const (
	linearProfileProvider = "linear-profile"
	linearGraphQLPath     = "/graphql"
)

type linearProxyContext struct {
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	EmployeeID    uuid.UUID
	ProfileID     uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	StatusCode    int
}

type LinearProxyHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
}

func NewLinearProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *LinearProxyHandler {
	return &LinearProxyHandler{db: db, encKey: encKey, nango: nangoClient}
}

// Handle proxies POST /internal/linear-proxy/{agentID} to Linear GraphQL
// through the employee's active linear-profile Nango connection.
func (h *LinearProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	eventCtx := linearProxyContext{Method: r.Method}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "linear proxy only supports POST"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	eventCtx.CallerAgentID = agentID

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		h.captureProxyFailure(ctx, eventCtx, http.StatusUnauthorized, "missing authorization")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent not found")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up agent")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up agent"})
		return
	}
	if agent.OrgID == nil {
		h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent has no org")
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent has no org"})
		return
	}
	eventCtx.OrgID = *agent.OrgID

	if !h.authenticatedSandbox(ctx, agentID, bearerToken) {
		h.captureProxyFailure(ctx, eventCtx, http.StatusUnauthorized, "invalid credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	employee, err := h.resolveOwningEmployee(ctx, *agent.OrgID, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "agent is not attached to an employee")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent is not attached to an employee"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to resolve employee")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve employee"})
		return
	}
	eventCtx.EmployeeID = employee.ID

	profile, conn, providerConfigKey, err := h.resolveLinearProfileConnection(ctx, employee)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "no linear profile connected to employee")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no linear profile connected to employee"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up linear profile")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up linear profile"})
		return
	}
	eventCtx.ProfileID = profile.ID
	eventCtx.ConnectionID = conn.ID

	resp, err := h.nango.RawProxyRequest(ctx, r.Method, providerConfigKey, conn.NangoConnectionID, linearGraphQLPath, "", proxyRequestBody(r), r.Header.Get("Content-Type"))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "linear-proxy: nango proxy failed",
			"agent_id", agentID,
			"employee_id", employee.ID,
			"profile_id", profile.ID,
			"connection_id", conn.ID,
			"method", r.Method,
			"error", err,
		)
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadGateway, "nango proxy failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "linear proxy request failed"})
		return
	}

	eventCtx.StatusCode = resp.StatusCode
	if resp.StatusCode >= http.StatusBadRequest {
		h.captureProxyFailure(ctx, eventCtx, resp.StatusCode, "linear upstream returned error")
	}
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *LinearProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return false
	}
	for _, sb := range sandboxes {
		decryptedKey, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decryptedKey)) == 1 {
			return true
		}
	}
	return false
}

func (h *LinearProxyHandler) resolveOwningEmployee(ctx context.Context, orgID uuid.UUID, agent model.Agent) (model.Agent, error) {
	if agent.IsEmployee {
		return agent, nil
	}
	var employee model.Agent
	if err := h.db.WithContext(ctx).
		Joins("JOIN agent_subagents ON agent_subagents.agent_id = agents.id").
		Where("agent_subagents.subagent_id = ? AND agents.org_id = ? AND agents.is_employee = ?", agent.ID, orgID, true).
		Order("agent_subagents.created_at ASC").
		First(&employee).Error; err != nil {
		return model.Agent{}, err
	}
	return employee, nil
}

func (h *LinearProxyHandler) resolveLinearProfileConnection(ctx context.Context, employee model.Agent) (model.AgentProfile, model.InConnection, string, error) {
	if employee.OrgID == nil {
		return model.AgentProfile{}, model.InConnection{}, "", gorm.ErrRecordNotFound
	}
	var profile model.AgentProfile
	if err := h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ? AND provider = ? AND status = ? AND revoked_at IS NULL AND deleted_at IS NULL", employee.ID, *employee.OrgID, linearProfileProvider, "active").
		First(&profile).Error; err != nil {
		return model.AgentProfile{}, model.InConnection{}, "", err
	}
	providerConfigKey := stringFromJSON(profile.Config, "provider_config_key")
	connectionID, err := uuid.Parse(stringFromJSON(profile.Config, "in_connection_id"))
	if err != nil || providerConfigKey == "" {
		return model.AgentProfile{}, model.InConnection{}, "", fmt.Errorf("linear profile is missing connection metadata")
	}

	var conn model.InConnection
	if err := h.db.WithContext(ctx).
		Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, *employee.OrgID).
		First(&conn).Error; err != nil {
		return model.AgentProfile{}, model.InConnection{}, "", err
	}
	if conn.InIntegration.Provider != linearProfileProvider {
		return model.AgentProfile{}, model.InConnection{}, "", fmt.Errorf("linear profile connection provider mismatch")
	}
	if conn.NangoConnectionID == "" {
		return model.AgentProfile{}, model.InConnection{}, "", fmt.Errorf("linear profile connection missing nango connection id")
	}
	return profile, conn, providerConfigKey, nil
}

func (h *LinearProxyHandler) captureProxyFailure(ctx context.Context, eventCtx linearProxyContext, status int, reason string) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("linear_proxy", "true")
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.CallerAgentID != uuid.Nil {
			scope.SetTag("agent_id", eventCtx.CallerAgentID.String())
		}
		if eventCtx.EmployeeID != uuid.Nil {
			scope.SetTag("employee_id", eventCtx.EmployeeID.String())
		}
		if eventCtx.ProfileID != uuid.Nil {
			scope.SetTag("profile_id", eventCtx.ProfileID.String())
		}
		if eventCtx.ConnectionID != uuid.Nil {
			scope.SetTag("connection_id", eventCtx.ConnectionID.String())
		}
		if status >= http.StatusInternalServerError {
			scope.SetLevel(sentrygo.LevelError)
		} else {
			scope.SetLevel(sentrygo.LevelWarning)
		}
		hub.CaptureException(fmt.Errorf("linear proxy %d: %s", status, reason))
	})
}
