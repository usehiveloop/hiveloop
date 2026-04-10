package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// AgentTriggerHandler handles CRUD for agent webhook triggers.
type AgentTriggerHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

// NewAgentTriggerHandler creates a new AgentTriggerHandler.
func NewAgentTriggerHandler(db *gorm.DB, catalog *catalog.Catalog) *AgentTriggerHandler {
	return &AgentTriggerHandler{db: db, catalog: catalog}
}

// --- Request / Response DTOs ---

type createAgentTriggerRequest struct {
	ConnectionID   string                `json:"connection_id"`
	TriggerKeys    []string              `json:"trigger_keys"`
	Enabled        *bool                 `json:"enabled,omitempty"`
	Conditions     *model.TriggerMatch   `json:"conditions,omitempty"`
	ContextActions []model.ContextAction `json:"context_actions,omitempty"`
	Instructions   string                `json:"instructions,omitempty"`
	TerminateOn    []model.TerminateRule `json:"terminate_on,omitempty"`
}

type updateAgentTriggerRequest struct {
	Enabled        *bool                 `json:"enabled,omitempty"`
	Conditions     *model.TriggerMatch   `json:"conditions,omitempty"`
	ContextActions []model.ContextAction `json:"context_actions,omitempty"`
	Instructions   *string               `json:"instructions,omitempty"`
	TerminateOn    []model.TerminateRule `json:"terminate_on,omitempty"`
}

type agentTriggerResponse struct {
	ID             string                `json:"id"`
	AgentID        string                `json:"agent_id"`
	ConnectionID   string                `json:"connection_id"`
	Provider       string                `json:"provider"`
	TriggerKeys    []string              `json:"trigger_keys"`
	Enabled        bool                  `json:"enabled"`
	Conditions     *model.TriggerMatch   `json:"conditions"`
	ContextActions []model.ContextAction `json:"context_actions"`
	Instructions   string                `json:"instructions"`
	TerminateOn    []model.TerminateRule `json:"terminate_on"`
	CreatedAt      string                `json:"created_at"`
	UpdatedAt      string                `json:"updated_at"`
}

func toAgentTriggerResponse(trigger model.AgentTrigger, provider string) agentTriggerResponse {
	resp := agentTriggerResponse{
		ID:           trigger.ID.String(),
		AgentID:      trigger.AgentID.String(),
		ConnectionID: trigger.ConnectionID.String(),
		Provider:     provider,
		TriggerKeys:  trigger.TriggerKeys,
		Enabled:      trigger.Enabled,
		Instructions: trigger.Instructions,
		CreatedAt:    trigger.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    trigger.UpdatedAt.Format(time.RFC3339),
	}

	if len(trigger.Conditions) > 0 {
		var match model.TriggerMatch
		if err := json.Unmarshal(trigger.Conditions, &match); err == nil {
			resp.Conditions = &match
		}
	}

	if len(trigger.ContextActions) > 0 {
		var actions []model.ContextAction
		if err := json.Unmarshal(trigger.ContextActions, &actions); err == nil {
			resp.ContextActions = actions
		}
	}
	if resp.ContextActions == nil {
		resp.ContextActions = []model.ContextAction{}
	}

	if len(trigger.TerminateOn) > 0 {
		var rules []model.TerminateRule
		if err := json.Unmarshal(trigger.TerminateOn, &rules); err == nil {
			resp.TerminateOn = rules
		}
	}
	if resp.TerminateOn == nil {
		resp.TerminateOn = []model.TerminateRule{}
	}

	return resp
}

// --- Validation helpers ---

var validConditionOperators = map[string]bool{
	"equals": true, "not_equals": true,
	"one_of": true, "not_one_of": true,
	"contains": true, "not_contains": true,
	"matches": true,
	"exists": true, "not_exists": true,
}

var validMatchModes = map[string]bool{
	"all": true, "any": true,
}

// validateConditions validates the trigger match conditions.
func validateConditions(conditions *model.TriggerMatch) string {
	if conditions == nil {
		return ""
	}
	if !validMatchModes[conditions.Mode] {
		return "conditions.mode must be \"all\" or \"any\""
	}
	for idx, cond := range conditions.Conditions {
		if cond.Path == "" {
			return fmt.Sprintf("conditions.conditions[%d].path is required", idx)
		}
		if !validConditionOperators[cond.Operator] {
			return fmt.Sprintf("conditions.conditions[%d].operator %q is not valid", idx, cond.Operator)
		}
		// exists/not_exists don't need a value.
		if cond.Operator != "exists" && cond.Operator != "not_exists" && cond.Value == nil {
			return fmt.Sprintf("conditions.conditions[%d].value is required for operator %q", idx, cond.Operator)
		}
	}
	return ""
}

// validateTerminateRules validates []TerminateRule and returns an error
// message on the first problem. Rules are rejected at save time if:
//
//   - any rule's trigger_keys is empty
//   - any trigger_key in a rule is also in the parent's TriggerKeys list (the
//     same event can't be both a normal trigger and a terminator — ambiguous)
//   - any trigger_key is not a known catalog trigger for the provider
//   - the rule's own conditions are malformed
//   - the rule's context_actions fail validation (same rules as parent context)
func validateTerminateRules(
	actionsCatalog *catalog.Catalog,
	rules []model.TerminateRule,
	parentTriggerKeys []string,
	provider string,
) string {
	if len(rules) == 0 {
		return ""
	}

	parentKeys := make(map[string]bool, len(parentTriggerKeys))
	for _, key := range parentTriggerKeys {
		parentKeys[key] = true
	}

	for idx, rule := range rules {
		if len(rule.TriggerKeys) == 0 {
			return fmt.Sprintf("terminate_on[%d].trigger_keys is required", idx)
		}
		for _, key := range rule.TriggerKeys {
			if key == "" {
				return fmt.Sprintf("terminate_on[%d].trigger_keys contains an empty string", idx)
			}
			if parentKeys[key] {
				return fmt.Sprintf("terminate_on[%d]: trigger key %q is also in the parent trigger_keys (ambiguous — an event can be either a normal trigger or a terminator, not both)", idx, key)
			}
		}
		// All keys must exist in the catalog for this provider.
		if err := actionsCatalog.ValidateTriggers(provider, rule.TriggerKeys); err != nil {
			// Try variant lookup before giving up.
			if _, ok := actionsCatalog.GetProviderTriggersForVariant(provider); !ok {
				return fmt.Sprintf("terminate_on[%d]: %s", idx, err.Error())
			}
		}
		// Rule conditions follow the same rules as normal trigger conditions.
		if rule.Conditions != nil {
			if errMsg := validateConditions(rule.Conditions); errMsg != "" {
				return fmt.Sprintf("terminate_on[%d].%s", idx, errMsg)
			}
		}
		// Rule context_actions follow the same rules as normal context_actions.
		if errMsg := validateContextActions(actionsCatalog, rule.ContextActions, provider); errMsg != "" {
			return fmt.Sprintf("terminate_on[%d].%s", idx, errMsg)
		}
	}
	return ""
}

// validateContextActions validates the context action list against the catalog.
func validateContextActions(actionsCatalog *catalog.Catalog, contextActions []model.ContextAction, provider string) string {
	if len(contextActions) == 0 {
		return ""
	}

	seenIDs := make(map[string]bool)
	for idx, contextAction := range contextActions {
		// Unique ID check.
		if contextAction.As == "" {
			return fmt.Sprintf("context_actions[%d].as is required", idx)
		}
		if seenIDs[contextAction.As] {
			return fmt.Sprintf("context_actions[%d].as %q is a duplicate", idx, contextAction.As)
		}
		seenIDs[contextAction.As] = true

		// Action must exist in catalog.
		if contextAction.Action == "" {
			return fmt.Sprintf("context_actions[%d].action is required", idx)
		}
		actionDef, ok := actionsCatalog.GetAction(provider, contextAction.Action)
		if !ok {
			return fmt.Sprintf("context_actions[%d].action %q does not exist for provider %q", idx, contextAction.Action, provider)
		}

		// Must be a read action.
		if actionDef.Access != catalog.AccessRead {
			return fmt.Sprintf("context_actions[%d].action %q is a write action; only read actions are allowed for context gathering", idx, contextAction.Action)
		}
	}

	return ""
}

// resolveProviderFromConnection resolves the provider name from a connection ID + org ID.
func resolveProviderFromConnection(db *gorm.DB, connectionID, orgID uuid.UUID) (string, error) {
	var connection model.Connection
	if err := db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, orgID).
		First(&connection).Error; err != nil {
		return "", err
	}
	return connection.Integration.Provider, nil
}

// validateTriggerRequest validates a trigger creation request without requiring an existing agent.
// agentIntegrations is the agent's Integrations JSON (connection IDs as keys).
// Returns (provider, errorMessage). If errorMessage is non-empty, validation failed.
func validateTriggerRequest(db *gorm.DB, actionsCatalog *catalog.Catalog, req *createAgentTriggerRequest, orgID uuid.UUID, agentIntegrations model.JSON) (string, string) {
	if req.ConnectionID == "" || len(req.TriggerKeys) == 0 {
		return "", "trigger.connection_id and trigger.trigger_keys are required"
	}

	connectionID, err := uuid.Parse(req.ConnectionID)
	if err != nil {
		return "", "trigger.connection_id is not a valid UUID"
	}

	// Verify the connection is in the agent's integrations.
	if _, exists := agentIntegrations[req.ConnectionID]; !exists {
		return "", "trigger connection is not configured for this agent; add it in the integrations step first"
	}

	// Resolve provider from connection.
	provider, err := resolveProviderFromConnection(db, connectionID, orgID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "trigger connection not found or revoked"
		}
		return "", "failed to validate trigger connection"
	}

	// Validate all trigger_keys exist in catalog for this provider.
	if err := actionsCatalog.ValidateTriggers(provider, req.TriggerKeys); err != nil {
		// Try variant lookup.
		if _, ok := actionsCatalog.GetProviderTriggersForVariant(provider); !ok {
			return "", err.Error()
		}
	}

	// Validate conditions.
	if errMsg := validateConditions(req.Conditions); errMsg != "" {
		return "", "trigger." + errMsg
	}

	// Validate context actions.
	if errMsg := validateContextActions(actionsCatalog, req.ContextActions, provider); errMsg != "" {
		return "", "trigger." + errMsg
	}

	// Validate terminate rules (including ambiguous-key detection against parent trigger keys).
	if errMsg := validateTerminateRules(actionsCatalog, req.TerminateOn, req.TriggerKeys, provider); errMsg != "" {
		return "", "trigger." + errMsg
	}

	return provider, ""
}

// --- CRUD Handlers ---

// Create handles POST /v1/agents/{agentID}/triggers
func (h *AgentTriggerHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")

	// Verify agent belongs to this org.
	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", agentID, org.ID).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find agent"})
		return
	}

	var req createAgentTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	provider, errMsg := validateTriggerRequest(h.db, h.catalog, &req, org.ID, agent.Integrations)
	if errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	connectionID, _ := uuid.Parse(req.ConnectionID)

	// Marshal JSONB fields.
	var conditionsJSON model.RawJSON
	if req.Conditions != nil {
		conditionsBytes, _ := json.Marshal(req.Conditions)
		conditionsJSON = model.RawJSON(conditionsBytes)
	}

	var contextActionsJSON model.RawJSON
	if len(req.ContextActions) > 0 {
		contextActionsBytes, _ := json.Marshal(req.ContextActions)
		contextActionsJSON = model.RawJSON(contextActionsBytes)
	}

	var terminateOnJSON model.RawJSON
	if len(req.TerminateOn) > 0 {
		terminateOnBytes, _ := json.Marshal(req.TerminateOn)
		terminateOnJSON = model.RawJSON(terminateOnBytes)
	}
	terminateEventKeys := pq.StringArray(model.CollectTerminateEventKeys(req.TerminateOn))

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	trigger := model.AgentTrigger{
		OrgID:              org.ID,
		AgentID:            agent.ID,
		ConnectionID:       connectionID,
		TriggerKeys:        req.TriggerKeys,
		Enabled:            enabled,
		Conditions:         conditionsJSON,
		ContextActions:     contextActionsJSON,
		Instructions:       req.Instructions,
		TerminateOn:        terminateOnJSON,
		TerminateEventKeys: terminateEventKeys,
	}

	if err := h.db.Create(&trigger).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "trigger with these keys already exists for this agent and connection"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create trigger"})
		return
	}

	// GORM ignores bool false as zero value during Create, so explicitly update if disabled.
	if !enabled {
		h.db.Model(&trigger).Update("enabled", false)
		trigger.Enabled = false
	}

	writeJSON(w, http.StatusCreated, toAgentTriggerResponse(trigger, provider))
}

// List handles GET /v1/agents/{agentID}/triggers
func (h *AgentTriggerHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")

	var triggers []model.AgentTrigger
	if err := h.db.Preload("Connection.Integration").
		Where("agent_id = ? AND org_id = ?", agentID, org.ID).
		Order("created_at DESC").
		Find(&triggers).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list triggers"})
		return
	}

	resp := make([]agentTriggerResponse, len(triggers))
	for idx, trigger := range triggers {
		provider := trigger.Connection.Integration.Provider
		resp[idx] = toAgentTriggerResponse(trigger, provider)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// Get handles GET /v1/agents/{agentID}/triggers/{id}
func (h *AgentTriggerHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	triggerID := chi.URLParam(r, "id")

	var trigger model.AgentTrigger
	if err := h.db.Preload("Connection.Integration").
		Where("id = ? AND agent_id = ? AND org_id = ?", triggerID, agentID, org.ID).
		First(&trigger).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get trigger"})
		return
	}

	provider := trigger.Connection.Integration.Provider
	writeJSON(w, http.StatusOK, toAgentTriggerResponse(trigger, provider))
}

// Update handles PUT /v1/agents/{agentID}/triggers/{id}
func (h *AgentTriggerHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	triggerID := chi.URLParam(r, "id")

	// Fetch existing trigger with connection for provider resolution.
	var trigger model.AgentTrigger
	if err := h.db.Preload("Connection.Integration").
		Where("id = ? AND agent_id = ? AND org_id = ?", triggerID, agentID, org.ID).
		First(&trigger).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find trigger"})
		return
	}

	provider := trigger.Connection.Integration.Provider

	var req updateAgentTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if req.Conditions != nil {
		if errMsg := validateConditions(req.Conditions); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		conditionsBytes, _ := json.Marshal(req.Conditions)
		updates["conditions"] = model.RawJSON(conditionsBytes)
	}

	if req.ContextActions != nil {
		if errMsg := validateContextActions(h.catalog, req.ContextActions, provider); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		contextActionsBytes, _ := json.Marshal(req.ContextActions)
		updates["context_actions"] = model.RawJSON(contextActionsBytes)
	}

	if req.Instructions != nil {
		updates["instructions"] = *req.Instructions
	}

	if req.TerminateOn != nil {
		// Load the current trigger_keys to cross-check for ambiguity.
		currentTriggerKeys := []string(trigger.TriggerKeys)
		if errMsg := validateTerminateRules(h.catalog, req.TerminateOn, currentTriggerKeys, provider); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
		terminateBytes, _ := json.Marshal(req.TerminateOn)
		updates["terminate_on"] = model.RawJSON(terminateBytes)
		updates["terminate_event_keys"] = pq.StringArray(model.CollectTerminateEventKeys(req.TerminateOn))
	}

	if len(updates) > 0 {
		if err := h.db.Model(&trigger).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update trigger"})
			return
		}
	}

	// Reload for response.
	h.db.Preload("Connection.Integration").Where("id = ?", trigger.ID).First(&trigger)
	writeJSON(w, http.StatusOK, toAgentTriggerResponse(trigger, provider))
}

// Delete handles DELETE /v1/agents/{agentID}/triggers/{id}
func (h *AgentTriggerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")
	triggerID := chi.URLParam(r, "id")

	result := h.db.Where("id = ? AND agent_id = ? AND org_id = ?", triggerID, agentID, org.ID).
		Delete(&model.AgentTrigger{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete trigger"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
