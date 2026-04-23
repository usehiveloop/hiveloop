package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// loadConversation loads and validates a conversation from the URL param + org context.
func (h *ConversationHandler) loadConversation(w http.ResponseWriter, r *http.Request) (*model.AgentConversation, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return nil, false
	}

	convID := chi.URLParam(r, "convID")
	var conv model.AgentConversation
	if err := h.db.Preload("Sandbox").Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return nil, false
	}

	if conv.Status != "active" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "conversation has ended"})
		return nil, false
	}

	return &conv, true
}

// getFlusher extracts http.Flusher from a ResponseWriter, unwrapping middleware wrappers if needed.
func getFlusher(w http.ResponseWriter) (http.Flusher, bool) {
	if f, ok := w.(http.Flusher); ok {
		return f, true
	}
	// Try to unwrap (chi middleware wraps ResponseWriter)
	type unwrapper interface {
		Unwrap() http.ResponseWriter
	}
	if u, ok := w.(unwrapper); ok {
		return getFlusher(u.Unwrap())
	}
	// Go 1.20+ http.ResponseController can flush any writer
	rc := http.NewResponseController(w)
	if rc.Flush() == nil {
		return &responseControllerFlusher{rc: rc}, true
	}
	return nil, false
}

// responseControllerFlusher wraps http.ResponseController as an http.Flusher.
type responseControllerFlusher struct {
	rc *http.ResponseController
}

func (f *responseControllerFlusher) Flush() {
	f.rc.Flush()
}

// getBridgeClient returns a Bridge client for the conversation's sandbox.
func (h *ConversationHandler) getBridgeClient(w http.ResponseWriter, r *http.Request, conv *model.AgentConversation) (*bridgepkg.BridgeClient, bool) {
	client, err := h.orchestrator.GetBridgeClient(r.Context(), &conv.Sandbox)
	if err != nil {
		slog.Error("failed to get bridge client", "conversation_id", conv.ID, "sandbox_id", conv.SandboxID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to sandbox"})
		return nil, false
	}
	return client, true
}

// strconvAtoiPositive parses a positive integer. Returns an error on non-positive values.
func strconvAtoiPositive(s string) (int, error) {
	n := 0
	if len(s) == 0 {
		return 0, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("not positive")
	}
	return n, nil
}
