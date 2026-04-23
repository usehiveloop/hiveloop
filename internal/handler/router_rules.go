package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
