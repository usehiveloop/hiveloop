package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// RouterHandler handles CRUD for Router, RouterTrigger, and RoutingRule.
type RouterHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

func NewRouterHandler(db *gorm.DB, actionsCatalog *catalog.Catalog) *RouterHandler {
	return &RouterHandler{db: db, catalog: actionsCatalog}
}

// --- Request / response types for swagger ---

// updateRouterRequest is the request body for updating a router.
type updateRouterRequest struct {
	Persona        *string `json:"persona,omitempty"`
	DefaultAgentID *string `json:"default_agent_id,omitempty"`
	MemoryTeam     *string `json:"memory_team,omitempty"`
}

// createTriggerRequest is the request body for creating a router trigger.
type createTriggerRequest struct {
	TriggerType           string   `json:"trigger_type"`            // "webhook" (default), "http", "cron"
	ConnectionID          string   `json:"connection_id,omitempty"` // required for webhook
	TriggerKeys           []string `json:"trigger_keys,omitempty"`  // required for webhook
	RoutingMode           string   `json:"routing_mode,omitempty"`  // "rule" (default) or "triage"
	EnrichCrossReferences bool     `json:"enrich_cross_references,omitempty"`
	CronSchedule          string   `json:"cron_schedule,omitempty"` // required for cron
	Instructions          string   `json:"instructions,omitempty"`
}

// createRuleRequest is the request body for creating a routing rule.
type createRuleRequest struct {
	AgentID    string          `json:"agent_id"`
	Priority   int             `json:"priority,omitempty"`
	Conditions json.RawMessage `json:"conditions,omitempty"`
}

// --- Router CRUD ---

// GetOrCreateRouter returns the org's router, creating one if it doesn't exist.
// @Summary Get or create router
// @Description Returns the organization's router, creating one automatically if it doesn't exist yet.
// @Tags router
// @Produce json
// @Success 200 {object} model.Router
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router [get]
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
// @Summary Update router
// @Description Updates the organization's router settings such as persona, default agent, and memory team.
// @Tags router
// @Accept json
// @Produce json
// @Param body body updateRouterRequest true "Router update fields"
// @Success 200 {object} model.Router
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router [put]
func (handler *RouterHandler) UpdateRouter(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body updateRouterRequest
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
			// Verify the agent belongs to the caller's org to prevent
			// cross-tenant routing (see security issue #48).
			var agent model.Agent
			if err := handler.db.Select("id").Where("id = ? AND org_id = ?", parsed, org.ID).First(&agent).Error; err != nil {
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
//
// Supported trigger_type values:
//   - "webhook" (default): requires connection_id and trigger_keys
//   - "http": no connection required; returns a unique URL for receiving requests
//   - "cron": requires cron_schedule; fires on schedule
//
// @Summary Create a router trigger
// @Description Creates a new trigger on the organization's router. Webhook triggers require a connection_id and trigger_keys. HTTP triggers generate a unique URL. Cron triggers require a cron_schedule expression.
// @Tags router
// @Accept json
// @Produce json
// @Param body body createTriggerRequest true "Trigger definition"
// @Success 201 {object} model.RouterTrigger
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers [post]
func (handler *RouterHandler) CreateTrigger(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body createTriggerRequest
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.TriggerType == "" {
		body.TriggerType = "webhook"
	}
	if body.RoutingMode == "" {
		body.RoutingMode = "rule"
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

	trigger := model.RouterTrigger{
		OrgID:                 org.ID,
		RouterID:              router.ID,
		Enabled:               true,
		TriggerType:           body.TriggerType,
		RoutingMode:           body.RoutingMode,
		EnrichCrossReferences: body.EnrichCrossReferences,
		Instructions:          body.Instructions,
	}

	switch body.TriggerType {
	case "webhook":
		if body.ConnectionID == "" || len(body.TriggerKeys) == 0 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "connection_id and trigger_keys are required for webhook triggers"})
			return
		}
		connectionID, err := uuid.Parse(body.ConnectionID)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid connection_id"})
			return
		}
		// Verify the connection belongs to the caller's org to prevent
		// cross-tenant trigger references (see security issue #54).
		var connection model.InConnection
		if err := handler.db.Select("id").Where("id = ? AND org_id = ?", connectionID, org.ID).First(&connection).Error; err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid connection_id"})
			return
		}
		trigger.ConnectionID = &connectionID
		trigger.TriggerKeys = body.TriggerKeys

	case "http":
		// No connection required. TriggerKeys are optional for HTTP triggers.
		trigger.TriggerKeys = body.TriggerKeys

	case "cron":
		if body.CronSchedule == "" {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "cron_schedule is required for cron triggers"})
			return
		}
		nextRun, err := computeNextRun(body.CronSchedule)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid cron_schedule: " + err.Error()})
			return
		}
		trigger.CronSchedule = body.CronSchedule
		trigger.NextRunAt = &nextRun

	default:
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "trigger_type must be 'webhook', 'http', or 'cron'"})
		return
	}

	if err := handler.db.Create(&trigger).Error; err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create trigger"})
		return
	}

	writeJSON(writer, http.StatusCreated, trigger)
}

// ListTriggers lists all router triggers for the org.
// @Summary List router triggers
// @Description Returns all triggers configured on the organization's router.
// @Tags router
// @Produce json
// @Success 200 {array} model.RouterTrigger
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers [get]
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
// @Summary Delete a router trigger
// @Description Removes a trigger from the organization's router.
// @Tags router
// @Produce json
// @Param id path string true "Trigger ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers/{id} [delete]
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
// @Summary Create a routing rule
// @Description Adds a routing rule to a trigger. Rules determine which agent handles events that match the trigger.
// @Tags router
// @Accept json
// @Produce json
// @Param id path string true "Trigger ID"
// @Param body body createRuleRequest true "Rule definition"
// @Success 201 {object} model.RoutingRule
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers/{id}/rules [post]
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

	var body createRuleRequest
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	agentID, err := uuid.Parse(body.AgentID)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	// Verify the agent belongs to the caller's org to prevent cross-tenant
	// routing rules (see security issue #49).
	var agent model.Agent
	if err := handler.db.Select("id").Where("id = ? AND org_id = ?", agentID, org.ID).First(&agent).Error; err != nil {
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
// @Summary List routing rules
// @Description Returns all routing rules for a specific trigger, ordered by priority.
// @Tags router
// @Produce json
// @Param id path string true "Trigger ID"
// @Success 200 {array} model.RoutingRule
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers/{id}/rules [get]
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
// @Summary Delete a routing rule
// @Description Removes a routing rule from a trigger.
// @Tags router
// @Produce json
// @Param id path string true "Trigger ID"
// @Param ruleID path string true "Rule ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/triggers/{id}/rules/{ruleID} [delete]
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
// @Summary List routing decisions
// @Description Returns the most recent routing decisions for the organization, useful for auditing and debugging trigger routing.
// @Tags router
// @Produce json
// @Success 200 {array} model.RoutingDecision
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/router/decisions [get]
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

// computeNextRun parses a cron expression and returns the next fire time.
func computeNextRun(cronExpr string) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(time.Now()), nil
}
