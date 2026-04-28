package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func mountConnections(r chi.Router, st *store) {
	r.Post("/connections", createConnection(st))
	r.Get("/connections/{id}", readConnection(st))
	r.Delete("/connections/{id}", deleteConnectionH(st))

	// legacy aliases — still served by real Nango.
	r.Post("/connection", createConnection(st))
	r.Get("/connection/{id}", readConnection(st))
	r.Delete("/connection/{id}", deleteConnectionH(st))
}

type createConnectionReq struct {
	ProviderConfigKey string `json:"provider_config_key"`
	ConnectionID      string `json:"connection_id"`
	APIKey            string `json:"api_key,omitempty"`
}

func createConnection(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createConnectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if req.ConnectionID == "" {
			req.ConnectionID = newID()
		}
		creds := map[string]any{"api_key": req.APIKey}
		c := &connection{
			ID:                req.ConnectionID,
			ProviderConfigKey: req.ProviderConfigKey,
			Provider:          providerFromKey(st, req.ProviderConfigKey),
			Credentials:       creds,
			ConnectionConfig:  map[string]any{},
		}
		st.putConnection(c)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "connectionId": c.ID})
	}
}

func readConnection(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		c, ok := st.getConnection(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"connection_id":       c.ID,
			"provider_config_key": c.ProviderConfigKey,
			"provider":            c.Provider,
			"credentials":         c.Credentials,
			"connection_config":   c.ConnectionConfig,
		})
	}
}

func deleteConnectionH(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.deleteConnection(chi.URLParam(r, "id"))
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

func providerFromKey(st *store, key string) string {
	if i, ok := st.getIntegration(key); ok {
		return i.Provider
	}
	return ""
}
