package main

import (
	"net/http"
)

func oauthCallback(st *store, h *hub, wh *webhookSender) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		sess, ok := st.takeOAuthSession(state)
		if !ok {
			renderAuthHTML(w, "invalid_state")
			return
		}

		out := st.consumeOutcome()
		if out.Result == "reject" {
			h.sendError(sess.WSClientID, sess.ProviderConfigKey, sess.ConnectionID,
				out.ErrorType, out.ErrorDesc)
			renderAuthHTML(w, "rejected")
			return
		}

		creds := out.Credentials
		if creds == nil {
			creds = defaultCreds(sess)
		}
		conn := &connection{
			ID:                sess.ConnectionID,
			ProviderConfigKey: sess.ProviderConfigKey,
			Provider:          sess.Provider,
			Credentials:       creds,
			ConnectionConfig:  map[string]any{},
		}
		st.putConnection(conn)

		notifyAndWebhook(h, wh, st, sess.ProviderConfigKey, conn, sess.WSClientID, sess.AuthMode)
		renderAuthHTML(w, "")
	}
}

func defaultCreds(sess *oauthSession) map[string]any {
	switch sess.AuthMode {
	case "APP":
		return map[string]any{
			"access_token":    "ghs_fake_" + newID(),
			"installation_id": "42",
			"type":            "APP",
		}
	default:
		return map[string]any{
			"access_token":  "fake_token_" + newID(),
			"refresh_token": "fake_refresh_" + newID(),
			"type":          "OAUTH2",
		}
	}
}

func notifyAndWebhook(h *hub, wh *webhookSender, st *store, providerConfigKey string, conn *connection, wsClientID, authMode string) {
	if wsClientID != "" {
		h.sendSuccess(wsClientID, providerConfigKey, conn.ID)
	}
	wh.fireAuth(authWebhook{
		From:              "nango",
		Type:              "auth",
		ConnectionID:      conn.ID,
		AuthMode:          authMode,
		ProviderConfigKey: providerConfigKey,
		Provider:          conn.Provider,
		Environment:       "dev",
		Operation:         "creation",
		Success:           true,
	})
	st.recordCall("WEBHOOK", "auth/"+providerConfigKey)
}
