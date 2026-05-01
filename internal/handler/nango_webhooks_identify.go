package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *NangoWebhookHandler) identify(wh *nangoWebhook) *webhookContext {
	if strings.HasPrefix(wh.ProviderConfigKey, "in_") {
		return h.identifyInIntegration(wh)
	}

	orgID, _, ok := parseProviderConfigKey(wh.ProviderConfigKey)
	if !ok {
		slog.Warn("nango webhook: unable to parse provider config key",
			"provider_config_key", wh.ProviderConfigKey,
			"type", wh.Type,
		)
		return nil
	}

	wctx := &webhookContext{orgID: orgID}

	var inConnection model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("nango_connection_id = ? AND org_id = ? AND revoked_at IS NULL",
			wh.ConnectionID, orgID).First(&inConnection).Error; err != nil {
		return wctx
	}
	wctx.inConnection = &inConnection

	return wctx
}

func (h *NangoWebhookHandler) identifyInIntegration(wh *nangoWebhook) *webhookContext {
	var inConnection model.InConnection
	err := h.db.Preload("InIntegration").
		Where("nango_connection_id = ? AND revoked_at IS NULL", wh.ConnectionID).
		Order("created_at DESC").
		First(&inConnection).Error
	if err != nil {
		return nil
	}

	return &webhookContext{
		orgID:        inConnection.OrgID,
		inConnection: &inConnection,
	}
}

func parseProviderConfigKey(key string) (uuid.UUID, string, bool) {
	parts := strings.SplitN(key, "_", 2)
	if len(parts) != 2 {
		return uuid.Nil, "", false
	}
	orgID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	return orgID, parts[1], true
}

func verifyNangoSignature(body []byte, secret string, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
