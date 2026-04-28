package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func mountIntegrations(r chi.Router, st *store) {
	r.Post("/integrations", createIntegration(st))
	r.Get("/integrations/{key}", getIntegration(st))
	r.Patch("/integrations/{key}", updateIntegration(st))
	r.Delete("/integrations/{key}", deleteIntegration(st))
}

type createIntegrationReq struct {
	UniqueKey   string         `json:"unique_key"`
	Provider    string         `json:"provider"`
	DisplayName string         `json:"display_name"`
	Credentials map[string]any `json:"credentials,omitempty"`
}

func createIntegration(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createIntegrationReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		integ := &integration{
			UniqueKey:     req.UniqueKey,
			Provider:      req.Provider,
			DisplayName:   req.DisplayName,
			Credentials:   req.Credentials,
			WebhookURL:    "https://fake-nango.local/webhook/" + req.UniqueKey,
			WebhookSecret: deriveWebhookSecret(req.UniqueKey),
		}
		if integ.Credentials == nil {
			integ.Credentials = map[string]any{}
		}
		if t, _ := integ.Credentials["type"].(string); t == "APP" {
			integ.Credentials["webhook_secret"] = integ.WebhookSecret
		}
		st.putIntegration(integ)
		writeJSON(w, http.StatusOK, apiResp{Data: integration2payload(integ)})
	}
}

func getIntegration(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i, ok := st.getIntegration(chi.URLParam(r, "key"))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusOK, apiResp{Data: integration2payload(i)})
	}
}

func updateIntegration(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i, ok := st.getIntegration(chi.URLParam(r, "key"))
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "integration not found"})
			return
		}
		var patch struct {
			DisplayName string         `json:"display_name,omitempty"`
			Credentials map[string]any `json:"credentials,omitempty"`
		}
		_ = json.NewDecoder(r.Body).Decode(&patch)
		if patch.DisplayName != "" {
			i.DisplayName = patch.DisplayName
		}
		if patch.Credentials != nil {
			i.Credentials = patch.Credentials
			if t, _ := i.Credentials["type"].(string); t == "APP" {
				i.WebhookSecret = newID()
				i.Credentials["webhook_secret"] = i.WebhookSecret
			}
		}
		st.putIntegration(i)
		writeJSON(w, http.StatusOK, apiResp{Data: integration2payload(i)})
	}
}

func deleteIntegration(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st.deleteIntegration(chi.URLParam(r, "key"))
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}

func integration2payload(i *integration) map[string]any {
	return map[string]any{
		"unique_key":       i.UniqueKey,
		"provider":         i.Provider,
		"display_name":     i.DisplayName,
		"credentials":      i.Credentials,
		"webhook_url":      i.WebhookURL,
		"forward_webhooks": true,
	}
}

func deriveWebhookSecret(key string) string {
	return "whsec_fake_" + key
}
