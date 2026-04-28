package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func mountProxy(r chi.Router, st *store) {
	r.HandleFunc("/proxy/*", proxyHandler(st))
}

func proxyHandler(st *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/proxy")
		if path == "" {
			path = "/"
		}
		f, ok := st.findFixture(r.Method, path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error":   "no fixture",
				"method":  r.Method,
				"path":    path,
				"hint":    "POST a scenario via /_admin/load or /_admin/fixtures",
			})
			return
		}
		for k, v := range f.Headers {
			w.Header().Set(k, v)
		}
		writeJSON(w, f.Status, f.Body)
	}
}
