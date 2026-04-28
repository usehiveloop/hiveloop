package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func mountConnectSessions(r chi.Router, st *store) {
	r.Post("/connect/sessions", createSession(st))
	r.Post("/connect/sessions/reconnect", createReconnect(st))
	r.Get("/connect/session", getSession(st))
	r.Delete("/connect/session", deleteSession(st))
}

type createSessionReq struct {
	EndUser struct {
		ID string `json:"id"`
	} `json:"end_user"`
	AllowedIntegrations []string `json:"allowed_integrations,omitempty"`
}

func createSession(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createSessionReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		token := "csess_" + newID()
		st.putConnectSession(&connectSession{
			Token:               token,
			EndUserID:           req.EndUser.ID,
			AllowedIntegrations: req.AllowedIntegrations,
			CreatedAt:           time.Now(),
		})
		expires := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
		writeJSON(w, http.StatusCreated, apiResp{Data: map[string]any{
			"token":        token,
			"connect_link": "http://localhost:3004/?session_token=" + token,
			"expires_at":   expires,
		}})
	}
}

type reconnectReq struct {
	ConnectionID  string `json:"connection_id"`
	IntegrationID string `json:"integration_id"`
}

func createReconnect(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reconnectReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		token := "csess_" + newID()
		st.putConnectSession(&connectSession{
			Token:                token,
			ExistingConnectionID: req.ConnectionID,
			AllowedIntegrations:  []string{req.IntegrationID},
			CreatedAt:            time.Now(),
		})
		expires := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
		writeJSON(w, http.StatusCreated, apiResp{Data: map[string]any{
			"token":      token,
			"expires_at": expires,
		}})
	}
}

func getSession(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("session_token")
		if token == "" {
			token = bearer(r)
		}
		sess, ok := st.getConnectSession(token)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusOK, apiResp{Data: map[string]any{
			"end_user":             map[string]string{"id": sess.EndUserID},
			"allowed_integrations": sess.AllowedIntegrations,
		}})
	}
}

func deleteSession(_ *store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}
