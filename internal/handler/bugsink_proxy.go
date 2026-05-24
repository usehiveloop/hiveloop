package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
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

const (
	bugsinkProvider           = "bugsink"
	bugsinkCanonicalAPIPrefix = "/api/canonical/0/"
)

type bugsinkProxyContext struct {
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	EmployeeID    uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	Path          string
	StatusCode    int
}

type BugsinkProxyHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	nango  *nango.Client
}

func NewBugsinkProxyHandler(db *gorm.DB, encKey *crypto.SymmetricKey, nangoClient *nango.Client) *BugsinkProxyHandler {
	return &BugsinkProxyHandler{db: db, encKey: encKey, nango: nangoClient}
}

func (h *BugsinkProxyHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID, path, forwardPath, ok := h.parseRequest(w, r)
	if !ok {
		return
	}
	eventCtx := bugsinkProxyContext{
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

	conn, err := h.resolveAttachedBugsinkConnection(ctx, employee)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.captureProxyFailure(ctx, eventCtx, http.StatusNotFound, "no bugsink connection attached to employee")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no bugsink connection attached to employee"})
			return
		}
		h.captureProxyFailure(ctx, eventCtx, http.StatusInternalServerError, "failed to look up bugsink connection")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up bugsink connection"})
		return
	}
	eventCtx.ConnectionID = conn.ID

	resp, err := h.nango.RawProxyRequest(ctx, r.Method, nangoProviderConfigKey(conn.Integration.UniqueKey), conn.NangoConnectionID, forwardPath, r.URL.RawQuery, proxyRequestBody(r), r.Header.Get("Content-Type"))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "bugsink-proxy: nango proxy failed",
			"employee_id", agentID,
			"employee_id", employee.ID,
			"connection_id", conn.ID,
			"path", path,
			"method", r.Method,
			"error", err,
		)
		h.captureProxyFailure(ctx, eventCtx, http.StatusBadGateway, "nango proxy failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "bugsink proxy request failed"})
		return
	}

	eventCtx.StatusCode = resp.StatusCode
	if resp.StatusCode >= http.StatusBadRequest {
		h.captureProxyFailure(ctx, eventCtx, resp.StatusCode, "bugsink upstream returned error")
	}
	copyProxyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (h *BugsinkProxyHandler) parseRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, string, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return uuid.Nil, "", "", false
	}
	path := "/" + strings.TrimLeft(chi.URLParam(r, "*"), "/")
	if !strings.HasPrefix(path, bugsinkCanonicalAPIPrefix) {
		h.captureProxyFailure(r.Context(), bugsinkProxyContext{CallerAgentID: agentID, Method: r.Method, Path: path}, http.StatusBadRequest, "invalid bugsink api path")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bugsink proxy only supports canonical api paths"})
		return uuid.Nil, "", "", false
	}
	forwardPath := "/" + strings.TrimLeft(strings.TrimPrefix(path, bugsinkCanonicalAPIPrefix), "/")
	return agentID, path, forwardPath, true
}

func (h *BugsinkProxyHandler) authenticatedSandbox(ctx context.Context, agentID uuid.UUID, bearerToken string) bool {
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

func (h *BugsinkProxyHandler) resolveOwningEmployee(ctx context.Context, orgID uuid.UUID, agent model.Employee) (model.Employee, error) {
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

func (h *BugsinkProxyHandler) resolveAttachedBugsinkConnection(ctx context.Context, employee model.Employee) (model.Connection, error) {
	if employee.OrgID == nil {
		return model.Connection{}, gorm.ErrRecordNotFound
	}
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", *employee.OrgID, bugsinkProvider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return model.Connection{}, err
	}
	return conn, nil
}

func (h *BugsinkProxyHandler) captureProxyFailure(ctx context.Context, eventCtx bugsinkProxyContext, status int, reason string) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("bugsink_proxy", "true")
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
		if eventCtx.Path != "" {
			scope.SetTag("bugsink.path", eventCtx.Path)
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
		hub.CaptureException(fmt.Errorf("bugsink proxy %d: %s", status, reason))
	})
}

func proxyRequestBody(r *http.Request) io.Reader {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil
	}
	return r.Body
}

func copyProxyHeaders(dst, src http.Header) {
	for key, vals := range src {
		if !safeProxyResponseHeader(key) {
			continue
		}
		for _, val := range vals {
			dst.Add(key, val)
		}
	}
}

func safeProxyResponseHeader(key string) bool {
	switch strings.ToLower(key) {
	case "content-type", "content-disposition", "link", "retry-after", "cache-control":
		return true
	}
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "ratelimit-")
}
