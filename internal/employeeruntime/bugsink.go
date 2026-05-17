package employeeruntime

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const bugsinkProvider = "bugsink"

// BugsinkDashboardBaseURL returns the real Bugsink instance base URL attached
// to the employee. It deliberately does not return BUGSINK_URL, because
// BUGSINK_URL is the Hiveloop proxy URL used for API calls.
func BugsinkDashboardBaseURL(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent model.Agent) string {
	if db == nil || orgID == uuid.Nil {
		return ""
	}
	connectionIDs := connectionIDsFromAgentIntegrations(agent.Integrations)
	if len(connectionIDs) == 0 {
		return ""
	}

	var conn model.InConnection
	if err := db.WithContext(ctx).
		Preload("InIntegration").
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.id IN ? AND in_connections.org_id = ? AND in_connections.revoked_at IS NULL AND in_integrations.provider = ?", connectionIDs, orgID, bugsinkProvider).
		Order("in_connections.created_at ASC").
		First(&conn).Error; err != nil {
		return ""
	}
	return BugsinkDashboardBaseURLFromConnection(conn)
}

func BugsinkDashboardBaseURLFromConnection(conn model.InConnection) string {
	return normalizeDashboardBaseURL(firstBugsinkString(
		conn.Meta["base_url"],
		nestedValue(conn.Meta, "connection_config", "base_url"),
		nestedValue(conn.Meta, "metadata", "base_url"),
	))
}

func connectionIDsFromAgentIntegrations(integrations model.JSON) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(integrations))
	for rawID := range integrations {
		id, err := uuid.Parse(rawID)
		if err == nil {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
	return ids
}

func nestedValue(values model.JSON, keys ...string) any {
	var current any = values
	for _, key := range keys {
		switch typed := current.(type) {
		case model.JSON:
			current = typed[key]
		case map[string]any:
			current = typed[key]
		default:
			return nil
		}
	}
	return current
}

func firstBugsinkString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func normalizeDashboardBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}
