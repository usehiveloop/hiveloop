package main

import (
	"encoding/json"
	"net/http"
)

type loadReq struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

func loadScenarioH(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loadReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		ref := req.Path
		if ref == "" {
			ref = req.Name
		}
		if ref == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name or path required"})
			return
		}
		sc, err := loadScenario(ref)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		applyScenario(st, sc)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "ok",
			"name":         sc.Name,
			"integrations": len(sc.Integrations),
			"connections":  len(sc.Connections),
			"fixtures":     len(sc.Proxy),
		})
	}
}

type setFixturesReq struct {
	Fixtures []scenarioFixture `json:"fixtures"`
}

func setFixturesH(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setFixturesReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		applyScenario(st, &scenarioYAML{Proxy: req.Fixtures})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "count": len(req.Fixtures)})
	}
}

func fireGitHub(st *store, wh *webhookSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req githubWebhookReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if req.EventType == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "event_type required"})
			return
		}
		body, headers := buildGitHubForward(req)
		wh.fireForward(req.Target, body, headers)
		st.recordCall("WEBHOOK", "github/"+req.EventType)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}
