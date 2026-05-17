package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type TriggerDeliveryHandler struct {
	db *gorm.DB
}

func NewTriggerDeliveryHandler(db *gorm.DB) *TriggerDeliveryHandler {
	return &TriggerDeliveryHandler{db: db}
}

type triggerDeliveryResponse struct {
	ID                    string          `json:"id"`
	AgentID               string          `json:"agent_id"`
	TriggerID             string          `json:"trigger_id"`
	ConnectionID           string          `json:"connection_id,omitempty"`
	DeliveryID            string          `json:"delivery_id"`
	EventKey              string          `json:"event_key"`
	ResourceKey           string          `json:"resource_key"`
	ConversationID        string          `json:"conversation_id"`
	RuntimeConversationID string          `json:"runtime_conversation_id"`
	RuntimeSessionID      string          `json:"runtime_session_id"`
	RuntimeStreamID       string          `json:"runtime_stream_id,omitempty"`
	RuntimeTraceID        string          `json:"runtime_trace_id,omitempty"`
	RuntimeTurnID         string          `json:"runtime_turn_id,omitempty"`
	Payload               json.RawMessage `json:"payload"`
	CreatedAt             string          `json:"created_at"`
}

func (h *TriggerDeliveryHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND agent_id = ?", org.ID, agentID)
	if triggerID := r.URL.Query().Get("trigger_id"); triggerID != "" {
		parsed, err := uuid.Parse(triggerID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid trigger_id"})
			return
		}
		q = q.Where("trigger_id = ?", parsed)
	}
	if eventKey := r.URL.Query().Get("event_key"); eventKey != "" {
		q = q.Where("event_key = ?", eventKey)
	}
	if resourceKey := r.URL.Query().Get("resource_key"); resourceKey != "" {
		q = q.Where("resource_key = ?", resourceKey)
	}
	q = applyPagination(q, cursor, limit)

	var rows []model.AgentTriggerDelivery
	if err := q.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list trigger deliveries"})
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	resp := make([]triggerDeliveryResponse, len(rows))
	for i, row := range rows {
		resp[i] = triggerDeliveryToResponse(row)
	}
	result := paginatedResponse[triggerDeliveryResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *TriggerDeliveryHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}
	deliveryID, err := uuid.Parse(chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid delivery ID"})
		return
	}
	var row model.AgentTriggerDelivery
	if err := h.db.Where("id = ? AND org_id = ? AND agent_id = ?", deliveryID, org.ID, agentID).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "trigger delivery not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load trigger delivery"})
		return
	}
	writeJSON(w, http.StatusOK, triggerDeliveryToResponse(row))
}

func triggerDeliveryToResponse(row model.AgentTriggerDelivery) triggerDeliveryResponse {
	var connectionID string
	if row.ConnectionID != nil {
		connectionID = row.ConnectionID.String()
	}
	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Time{}
	}
	return triggerDeliveryResponse{
		ID:                    row.ID.String(),
		AgentID:               row.AgentID.String(),
		TriggerID:             row.TriggerID.String(),
		ConnectionID:           connectionID,
		DeliveryID:            row.DeliveryID,
		EventKey:              row.EventKey,
		ResourceKey:           row.ResourceKey,
		ConversationID:        row.ConversationID.String(),
		RuntimeConversationID: row.RuntimeConversationID,
		RuntimeSessionID:      row.RuntimeSessionID,
		RuntimeStreamID:       row.RuntimeStreamID,
		RuntimeTraceID:        row.RuntimeTraceID,
		RuntimeTurnID:         row.RuntimeTurnID,
		Payload:               json.RawMessage(row.Payload),
		CreatedAt:             createdAt.Format(time.RFC3339),
	}
}
