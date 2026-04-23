package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *NangoWebhookHandler) identify(wh *nangoWebhook) *webhookContext {
	if strings.HasPrefix(wh.ProviderConfigKey, "in_") {
		return h.identifyInIntegration(wh)
	}

	orgID, uniqueKey, ok := parseProviderConfigKey(wh.ProviderConfigKey)
	if !ok {
		slog.Warn("nango webhook: unable to parse provider config key",
			"provider_config_key", wh.ProviderConfigKey,
			"type", wh.Type,
		)
		return nil
	}

	slog.Info("nango webhook: resolved org from config key",
		"org_id", orgID,
		"unique_key", uniqueKey,
	)

	wctx := &webhookContext{orgID: orgID}

	var inConnection model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("nango_connection_id = ? AND org_id = ? AND revoked_at IS NULL",
			wh.ConnectionID, orgID).First(&inConnection).Error; err != nil {
		slog.Warn("nango webhook: in-connection not found",
			"org_id", orgID,
			"nango_connection_id", wh.ConnectionID,
			"type", wh.Type,
			"error", err,
		)
		return wctx
	}
	wctx.inConnection = &inConnection

	logAttrs := []any{
		"type", wh.Type,
		"provider", inConnection.InIntegration.Provider,
		"org_id", orgID,
		"connection_id", inConnection.ID,
		"nango_connection_id", wh.ConnectionID,
	}
	if wh.Type == "auth" {
		logAttrs = append(logAttrs, "operation", wh.Operation)
		if wh.Success != nil {
			logAttrs = append(logAttrs, "success", *wh.Success)
		}
	}
	if wh.Type == "forward" {
		logAttrs = append(logAttrs, "payload_size", len(wh.Payload))
	}
	slog.Info("nango webhook: fully resolved", logAttrs...)

	return wctx
}

func (h *NangoWebhookHandler) identifyInIntegration(wh *nangoWebhook) *webhookContext {
	var inConnection model.InConnection
	err := h.db.Preload("InIntegration").
		Where("nango_connection_id = ? AND revoked_at IS NULL", wh.ConnectionID).
		Order("created_at DESC").
		First(&inConnection).Error
	if err != nil {
		slog.Warn("nango webhook: in-connection not found for in_* provider_config_key",
			"provider_config_key", wh.ProviderConfigKey,
			"nango_connection_id", wh.ConnectionID,
			"type", wh.Type,
			"operation", wh.Operation,
			"error", err,
		)
		return nil
	}

	slog.Info("nango webhook: resolved in-integration connection",
		"type", wh.Type,
		"provider_config_key", wh.ProviderConfigKey,
		"nango_connection_id", wh.ConnectionID,
		"in_connection_id", inConnection.ID,
		"in_integration_id", inConnection.InIntegrationID,
		"org_id", inConnection.OrgID,
		"provider", inConnection.InIntegration.Provider,
		"payload_size", len(wh.Payload),
	)

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
