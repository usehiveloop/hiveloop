package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func mountAdmin(r chi.Router, st *store, h *hub, wh *webhookSender) {
	r.Post("/_admin/outcome", setNextOutcome(st))
	r.Post("/_admin/reset", reset(st))
	r.Get("/_admin/log", dumpLog(st))
	r.Post("/_admin/webhook/forward", fireForward(st, wh))
	r.Post("/_admin/ws/notify", manualNotify(h))
	r.Post("/_admin/load", loadScenarioH(st))
	r.Post("/_admin/fixtures", setFixturesH(st))
	r.Post("/_admin/github-webhook", fireGitHub(st, wh))
}

type setOutcomeReq struct {
	// ProviderConfigKey scopes the outcome to a single integration's flow
	// (e.g. "in_github-2b548f11"). Omit to set the sticky default applied
	// when no per-key outcome is queued.
	ProviderConfigKey string         `json:"provider_config_key,omitempty"`
	Result            string         `json:"result"`
	ErrorType         string         `json:"error_type,omitempty"`
	ErrorDesc         string         `json:"error_desc,omitempty"`
	Credentials       map[string]any `json:"credentials,omitempty"`
}

func setNextOutcome(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setOutcomeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if req.Result == "" {
			req.Result = "approve"
		}
		st.setOutcome(req.ProviderConfigKey, outcome{
			Result:      req.Result,
			ErrorType:   req.ErrorType,
			ErrorDesc:   req.ErrorDesc,
			Credentials: req.Credentials,
		})
		scope := req.ProviderConfigKey
		if scope == "" {
			scope = "default"
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "scope": scope})
	}
}

func reset(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		st.reset()
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

func dumpLog(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"calls": st.snapshotCalls()})
	}
}

type forwardReq struct {
	Target            string            `json:"target,omitempty"`
	ConnectionID      string            `json:"connection_id"`
	ProviderConfigKey string            `json:"provider_config_key"`
	Provider          string            `json:"provider"`
	Payload           any               `json:"payload"`
	ProviderHeaders   map[string]string `json:"provider_headers,omitempty"`
}

func fireForward(st *store, wh *webhookSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req forwardReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		body := forwardWebhook{
			From:              req.Provider,
			Type:              "forward",
			ConnectionID:      req.ConnectionID,
			ProviderConfigKey: req.ProviderConfigKey,
			Payload:           req.Payload,
		}
		wh.fireForward(req.Target, body, req.ProviderHeaders)
		st.recordCall("WEBHOOK", "forward/"+req.ProviderConfigKey)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

type notifyReq struct {
	WSClientID        string `json:"ws_client_id"`
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id"`
	Result            string `json:"result"` // "success" | "error"
	ErrorType         string `json:"error_type,omitempty"`
}

func manualNotify(h *hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req notifyReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if req.Result == "error" {
			h.sendError(req.WSClientID, req.ProviderConfigKey, req.ConnectionID, req.ErrorType, "")
		} else {
			h.sendSuccess(req.WSClientID, req.ProviderConfigKey, req.ConnectionID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}
