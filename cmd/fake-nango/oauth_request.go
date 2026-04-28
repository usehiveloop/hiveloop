package main

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func mountOAuth(r chi.Router, st *store, h *hub, wh *webhookSender) {
	r.Get("/oauth/connect/{key}", oauthRequest(st, "OAUTH2"))
	r.Post("/oauth2/auth/{key}", oauth2CC(st, h, wh))
	r.Post("/auth/oauth-outbound/{key}", oauthOutbound(st, h, wh))
	r.Get("/oauth/callback", oauthCallback(st, h, wh))
}

func oauthRequest(st *store, defaultMode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		q := r.URL.Query()
		wsClientID := q.Get("ws_client_id")
		connID := q.Get("connection_id")
		if connID == "" {
			connID = newID()
		}

		mode := defaultMode
		if i, ok := st.getIntegration(key); ok {
			if t, _ := i.Credentials["type"].(string); t == "APP" {
				mode = "APP"
			}
		}

		state := newID()
		st.putOAuthSession(&oauthSession{
			State:             state,
			WSClientID:        wsClientID,
			ProviderConfigKey: key,
			Provider:          providerFromKey(st, key),
			AuthMode:          mode,
			ConnectionID:      connID,
		})

		// Skip the simulated provider redirect — call straight back to /oauth/callback
		// with the same shape a real provider would. Saves one hop and keeps the
		// agent flow deterministic.
		cb := url.URL{Path: "/oauth/callback"}
		params := cb.Query()
		params.Set("state", state)
		if mode == "APP" {
			params.Set("installation_id", "42")
			params.Set("setup_action", "install")
		} else {
			params.Set("code", "fake_authcode_"+state)
		}
		cb.RawQuery = params.Encode()
		http.Redirect(w, r, cb.String(), http.StatusFound)
	}
}

func oauth2CC(st *store, h *hub, wh *webhookSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		wsClientID := r.URL.Query().Get("ws_client_id")
		conn := completeConnection(st, key, "OAUTH2_CC", map[string]any{
			"access_token": "fake_cc_token_" + newID(),
		})
		notifyAndWebhook(h, wh, st, key, conn, wsClientID, "OAUTH2_CC")
		writeJSON(w, http.StatusOK, map[string]any{"connectionId": conn.ID, "providerConfigKey": key})
	}
}

func oauthOutbound(st *store, h *hub, wh *webhookSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		wsClientID := r.URL.Query().Get("ws_client_id")
		conn := completeConnection(st, key, "APP", map[string]any{
			"access_token":    "ghs_fake_outbound_" + newID(),
			"installation_id": "42",
		})
		notifyAndWebhook(h, wh, st, key, conn, wsClientID, "APP")
		writeJSON(w, http.StatusOK, map[string]any{"connectionId": conn.ID, "providerConfigKey": key})
	}
}

func completeConnection(st *store, providerConfigKey, _ string, creds map[string]any) *connection {
	c := &connection{
		ID:                newID(),
		ProviderConfigKey: providerConfigKey,
		Provider:          providerFromKey(st, providerConfigKey),
		Credentials:       creds,
		ConnectionConfig:  map[string]any{},
	}
	st.putConnection(c)
	return c
}
