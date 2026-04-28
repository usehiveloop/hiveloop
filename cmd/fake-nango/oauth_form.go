package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// mountFormAuth handles auth modes that don't open a popup. Each takes
// credentials in the JSON body and returns success synchronously, just
// like real Nango.
func mountFormAuth(r chi.Router, st *store, wh *webhookSender) {
	for _, p := range []string{
		"/api-auth/api-key/{key}",
		"/api-auth/basic/{key}",
		"/auth/jwt/{key}",
		"/auth/tba/{key}",
		"/auth/two-step/{key}",
		"/auth/bill/{key}",
		"/auth/signature/{key}",
		"/app-store-auth/{key}",
		"/auth/unauthenticated/{key}",
	} {
		r.Post(p, formAuthHandler(st, wh, p))
	}
}

func formAuthHandler(st *store, wh *webhookSender, route string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		mode := authModeForRoute(route)
		conn := completeConnection(st, key, mode, map[string]any{
			"type":    mode,
			"api_key": "fake_api_key_" + newID(),
		})
		wh.fireAuth(authWebhook{
			From:              "nango",
			Type:              "auth",
			ConnectionID:      conn.ID,
			AuthMode:          mode,
			ProviderConfigKey: key,
			Provider:          conn.Provider,
			Environment:       "dev",
			Operation:         "creation",
			Success:           true,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"connectionId":      conn.ID,
			"providerConfigKey": key,
		})
	}
}

func authModeForRoute(route string) string {
	switch route {
	case "/api-auth/api-key/{key}":
		return "API_KEY"
	case "/api-auth/basic/{key}":
		return "BASIC"
	case "/auth/jwt/{key}":
		return "JWT"
	case "/auth/tba/{key}":
		return "TBA"
	case "/auth/two-step/{key}":
		return "TWO_STEP"
	case "/auth/bill/{key}":
		return "BILL"
	case "/auth/signature/{key}":
		return "SIGNATURE"
	case "/app-store-auth/{key}":
		return "APP_STORE"
	case "/auth/unauthenticated/{key}":
		return "NONE"
	}
	return "UNKNOWN"
}
