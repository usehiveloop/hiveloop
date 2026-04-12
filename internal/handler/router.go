package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// RouterHandler handles CRUD for Router, RouterTrigger, and RoutingRule.
type RouterHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

// NewRouterHandler creates a new router handler.
func NewRouterHandler(db *gorm.DB, actionsCatalog *catalog.Catalog) *RouterHandler {
	return &RouterHandler{db: db, catalog: actionsCatalog}
}

// --- Router CRUD ---

// GetOrCreateRouter returns the org's router, creating one if it doesn't exist.
func (handler *RouterHandler) GetOrCreateRouter(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var router model.Router
	err := handler.db.Where("org_id = ?", org.ID).First(&router).Error
	if err == gorm.ErrRecordNotFound {
		router = model.Router{
			OrgID: org.ID,
			Name:  "Zira",
		}
		if createErr := handler.db.Create(&router).Error; createErr != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create router"})
			return
		}
	} else if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to load router"})
		return
	}

	writeJSON(writer, http.StatusOK, router)
}

// UpdateRouter updates the org's router (persona, default_agent, memory_team).
func (handler *RouterHandler) UpdateRouter(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body struct {
		Persona        *string `json:"persona"`
		DefaultAgentID *string `json:"default_agent_id"`
		MemoryTeam     *string `json:"memory_team"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var router model.Router
	if err := handler.db.Where("org_id = ?", org.ID).First(&router).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "router not found"})
		return
	}

	updates := map[string]any{}
	if body.Persona != nil {
		updates["persona"] = *body.Persona
	}
	if body.DefaultAgentID != nil {
		if *body.DefaultAgentID == "" {
			updates["default_agent_id"] = nil
		} else {
			parsed, err := uuid.Parse(*body.DefaultAgentID)
			if err != nil {
				writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid default_agent_id"})
				return
			}
			updates["default_agent_id"] = parsed
		}
	}
	if body.MemoryTeam != nil {
		updates["memory_team"] = *body.MemoryTeam
	}

	if len(updates) > 0 {
		handler.db.Model(&router).Updates(updates)
	}

	handler.db.Where("id = ?", router.ID).First(&router)
	writeJSON(writer, http.StatusOK, router)
}

// --- Router Trigger CRUD ---

// CreateTrigger creates a new router trigger.
func (handler *RouterHandler) CreateTrigger(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body struct {
		ConnectionID          string   `json:"connection_id"`
		TriggerKeys           []string `json:"trigger_keys"`
		RoutingMode           string   `json:"routing_mode"`
		EnrichCrossReferences bool     `json:"enrich_cross_references"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.ConnectionID == "" || len(body.TriggerKeys) == 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "connection_id and trigger_keys are required"})
		return
	}
	if body.RoutingMode == "" {
		body.RoutingMode = "triage"
	}
	if body.RoutingMode != "rule" && body.RoutingMode != "triage" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "routing_mode must be 'rule' or 'triage'"})
		return
	}

	var router model.Router
	if err := handler.db.Where("org_id = ?", org.ID).First(&router).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "router not found — create one first"})
		return
	}

	connectionID, err := uuid.Parse(body.ConnectionID)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid connection_id"})
		return
	}

	trigger := model.RouterTrigger{
		OrgID:                 org.ID,
		RouterID:              router.ID,
		ConnectionID:          connectionID,
		TriggerKeys:           body.TriggerKeys,
		Enabled:               true,
		RoutingMode:           body.RoutingMode,
		EnrichCrossReferences: body.EnrichCrossReferences,
	}
	if err := handler.db.Create(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create trigger"})
		return
	}

	writeJSON(writer, http.StatusCreated, trigger)
}

// ListTriggers lists all router triggers for the org.
func (handler *RouterHandler) ListTriggers(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var router model.Router
	if err := handler.db.Where("org_id = ?", org.ID).First(&router).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "router not found"})
		return
	}

	var triggers []model.RouterTrigger
	handler.db.Where("router_id = ?", router.ID).Find(&triggers)
	writeJSON(writer, http.StatusOK, triggers)
}

// DeleteTrigger deletes a router trigger by ID.
func (handler *RouterHandler) DeleteTrigger(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	triggerID := chi.URLParam(request, "id")
	result := handler.db.Where("id = ? AND org_id = ?", triggerID, org.ID).Delete(&model.RouterTrigger{})
	if result.RowsAffected == 0 {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "trigger not found"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Routing Rule CRUD ---

// CreateRule creates a routing rule on a trigger.
func (handler *RouterHandler) CreateRule(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	triggerID := chi.URLParam(request, "id")
	var trigger model.RouterTrigger
	if err := handler.db.Where("id = ? AND org_id = ?", triggerID, org.ID).First(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "trigger not found"})
		return
	}

	var body struct {
		AgentID    string          `json:"agent_id"`
		Priority   int             `json:"priority"`
		Conditions json.RawMessage `json:"conditions"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	agentID, err := uuid.Parse(body.AgentID)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	if body.Priority <= 0 {
		body.Priority = 1
	}

	rule := model.RoutingRule{
		RouterTriggerID: trigger.ID,
		AgentID:         agentID,
		Priority:        body.Priority,
		Conditions:      model.RawJSON(body.Conditions),
	}
	if err := handler.db.Create(&rule).Error; err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create rule"})
		return
	}

	writeJSON(writer, http.StatusCreated, rule)
}

// ListRules lists routing rules for a trigger.
func (handler *RouterHandler) ListRules(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	triggerID := chi.URLParam(request, "id")
	var trigger model.RouterTrigger
	if err := handler.db.Where("id = ? AND org_id = ?", triggerID, org.ID).First(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "trigger not found"})
		return
	}

	var rules []model.RoutingRule
	handler.db.Where("router_trigger_id = ?", trigger.ID).Order("priority ASC").Find(&rules)
	writeJSON(writer, http.StatusOK, rules)
}

// DeleteRule deletes a routing rule.
func (handler *RouterHandler) DeleteRule(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	ruleID := chi.URLParam(request, "ruleID")
	triggerID := chi.URLParam(request, "id")

	// Verify trigger belongs to org.
	var trigger model.RouterTrigger
	if err := handler.db.Where("id = ? AND org_id = ?", triggerID, org.ID).First(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "trigger not found"})
		return
	}

	result := handler.db.Where("id = ? AND router_trigger_id = ?", ruleID, trigger.ID).Delete(&model.RoutingRule{})
	if result.RowsAffected == 0 {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "rule not found"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Routing Decisions ---

// ListDecisions returns recent routing decisions for audit.
func (handler *RouterHandler) ListDecisions(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var decisions []model.RoutingDecision
	handler.db.Where("org_id = ?", org.ID).Order("created_at DESC").Limit(50).Find(&decisions)
	writeJSON(writer, http.StatusOK, decisions)
}
