package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

const notionProvider = "notion"

type notionProxyContext struct {
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	EmployeeID    uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	Path          string
	StatusCode    int
}

type NotionProxyHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
}

func NewNotionProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *NotionProxyHandler {
	return &NotionProxyHandler{db: db, encKey: encKey, nango: nangoClient}
}

func (h *NotionProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID, path, ok := h.parseRequest(w, r)
	if !ok {
		return
	}
	eventCtx := notionProxyContext{
		CallerAgentID: agentID,
		Method:        r.Method,
		Path:          path,
	}

	bearerToken := extractBearerToken(r)
	if bearerToken == "" {
		h.captureProxyFailure(ctx, eventCtx, http.StatusUnauthorized, "missing authorization")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}

	var agent model.Employee
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

	conn, providerConfigKey, err := h.resolveNotionConnection(ctx, employee)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "no notion connection for org")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no notion connection for org"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up notion connection")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up notion connection"})
		return
	}
	eventCtx.ConnectionID = conn.ID

	resp, err := h.nango.RawProxyRequestWithHeaders(ctx, r.Method, providerConfigKey, conn.NangoConnectionID, path, r.URL.RawQuery, proxyRequestBody(r), notionProxyHeaders(r))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "notion-proxy: nango proxy failed",
			"employee_id", agentID,
			"employee_id", employee.ID,
			"connection_id", conn.ID,
			"path", path,
			"method", r.Method,
			"error", err,
		)
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadGateway, "nango proxy failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "notion proxy request failed"})
		return
	}

	eventCtx.StatusCode = resp.StatusCode
	if resp.StatusCode >= http.StatusBadRequest {
		h.captureProxyFailure(ctx, eventCtx, resp.StatusCode, "notion upstream returned error")
	}
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *NotionProxyHandler) parseRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return uuid.Nil, "", false
	}
	path := "/" + strings.TrimLeft(chi.URLParam(r, "*"), "/")
	if !strings.HasPrefix(path, "/v1/") {
		h.captureProxyFailure(r.Context(), notionProxyContext{CallerAgentID: agentID, Method: r.Method, Path: path}, http.StatusBadRequest, "invalid notion api path")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "notion proxy only supports /v1 api paths"})
		return uuid.Nil, "", false
	}
	return agentID, path, true
}

func (h *NotionProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("employee_id = ?", agentID).Find(&sandboxes).Error; err != nil {
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

func (h *NotionProxyHandler) resolveOwningEmployee(ctx context.Context, orgID uuid.UUID, agent model.Employee) (model.Employee, error) {
	if agent.OrgID != nil && *agent.OrgID == orgID {
		return agent, nil
	}
	var employee model.Employee
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		First(&employee).Error; err != nil {
		return model.Employee{}, err
	}
	return employee, nil
}

func (h *NotionProxyHandler) resolveNotionConnection(ctx context.Context, employee model.Employee) (model.Connection, string, error) {
	if employee.OrgID == nil {
		return model.Connection{}, "", gorm.ErrRecordNotFound
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", *employee.OrgID, notionProvider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return model.Connection{}, "", err
	}
	if conn.NangoConnectionID == "" {
		return model.Connection{}, "", fmt.Errorf("notion connection missing nango connection id")
	}
	return conn, nangoProviderConfigKey(conn.Integration.UniqueKey), nil
}

func (h *NotionProxyHandler) captureProxyFailure(ctx context.Context, eventCtx notionProxyContext, status int, reason string) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("notion_proxy", "true")
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
		if eventCtx.Path != "" {
			scope.SetTag("notion.path", eventCtx.Path)
		}
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.CallerAgentID != uuid.Nil {
			scope.SetTag("employee_id", eventCtx.CallerAgentID.String())
		}
		if eventCtx.EmployeeID != uuid.Nil {
			scope.SetTag("employee_id", eventCtx.EmployeeID.String())
		}
		if eventCtx.ConnectionID != uuid.Nil {
			scope.SetTag("connection_id", eventCtx.ConnectionID.String())
		}
		if status >= http.StatusInternalServerError {
			scope.SetLevel(sentrygo.LevelError)
		} else {
			scope.SetLevel(sentrygo.LevelWarning)
		}
		hub.CaptureException(fmt.Errorf("notion proxy %d: %s", status, reason))
	})
}

func notionProxyHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for _, key := range []string{"Accept", "Content-Type", "Notion-Version"} {
		if val := r.Header.Get(key); val != "" {
			headers[key] = val
		}
	}
	return headers
}
