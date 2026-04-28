package main

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed providers.json
var providersJSON []byte

func mountProviders(r chi.Router, _ *store) {
	r.Get("/providers.json", servePublicProviders)
	r.Get("/providers", servePublicProviders)
	r.Get("/providers/{name}", serveProvider)
}

func servePublicProviders(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(providersJSON)
}

func serveProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var raw struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(providersJSON, &raw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, p := range raw.Data {
		if p["name"] == name {
			writeJSON(w, http.StatusOK, apiResp{Data: p})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown provider"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
