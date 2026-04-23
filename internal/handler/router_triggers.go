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
