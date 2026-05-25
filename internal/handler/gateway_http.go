package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/gateway"
)

type GatewayHTTPHandler struct {
	service  *gateway.Service
	maxBytes int64
}

func NewGatewayHTTPHandler(service *gateway.Service) *GatewayHTTPHandler {
	return &GatewayHTTPHandler{
		service:  service,
		maxBytes: 512 * 1024,
	}
}

func (h *GatewayHTTPHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "http gateway is not configured"})
		return
	}
	routeID, err := uuid.Parse(chi.URLParam(r, "routeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route_id"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	result, err := h.service.ReceiveWebhook(r.Context(), gateway.WebhookEnvelope{
		RouteID: routeID,
		Headers: normalizedHeaders(r.Header),
		Body:    body,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if result == nil || result.Ignored {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
		return
	}
	status := "delivered"
	code := http.StatusAccepted
	if result.Duplicate {
		status = "duplicate"
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{
		"status":              status,
		"event_id":            result.Event.ID.String(),
		"employee_session_id": result.Session.ID.String(),
		"runtime_session_id":  result.Runtime.SessionID,
		"runtime_stream_id":   result.Runtime.StreamID,
		"runtime_trace_id":    result.Runtime.TraceID,
		"runtime_turn_id":     result.Runtime.TurnID,
	})
}

func normalizedHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(key)] = values[0]
	}
	return out
}
