// Package mcp provides types and validation for integration-scoped tokens.
package mcp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/model"
)

// TokenScope represents a single scope rule for a minted token, granting
// access to specific actions on a specific connection.
type TokenScope struct {
	ConnectionID string              `json:"connection_id"`
	Actions      []string            `json:"actions"`
	Resources    map[string][]string `json:"resources,omitempty"`
}

// ValidateScopes checks all scope rules against the catalog and database.
// Each connection must exist, belong to the org, and not be revoked.
// Each action must exist in the catalog for the connection's provider.
// Wildcard actions are explicitly rejected.
func ValidateScopes(db *gorm.DB, orgID uuid.UUID, cat *catalog.Catalog, scopes []TokenScope) error {
	if len(scopes) == 0 {
		return nil
	}

	for i, scope := range scopes {
		if scope.ConnectionID == "" {
			return fmt.Errorf("scope[%d]: connection_id is required", i)
		}

		connUUID, err := uuid.Parse(scope.ConnectionID)
		if err != nil {
			return fmt.Errorf("scope[%d]: invalid connection_id", i)
		}

		if len(scope.Actions) == 0 {
			return fmt.Errorf("scope[%d]: actions must not be empty", i)
		}

		// Verify connection exists, belongs to org, and is not revoked
		var conn model.Connection
		if err := db.Preload("Integration").
			Where("id = ? AND org_id = ? AND revoked_at IS NULL", connUUID, orgID).
			First(&conn).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fmt.Errorf("scope[%d]: connection %q not found or revoked", i, scope.ConnectionID)
			}
			return fmt.Errorf("scope[%d]: failed to verify connection: %w", i, err)
		}

		// Check the integration is not soft-deleted
		if conn.Integration.DeletedAt != nil {
			return fmt.Errorf("scope[%d]: integration for connection %q has been deleted", i, scope.ConnectionID)
		}

		provider := conn.Integration.Provider

		// Validate actions against catalog
		if err := cat.ValidateActions(provider, scope.Actions); err != nil {
			return fmt.Errorf("scope[%d]: %w", i, err)
		}

		// Validate resource types
		if err := cat.ValidateResources(provider, scope.Actions, scope.Resources); err != nil {
			return fmt.Errorf("scope[%d]: %w", i, err)
		}
	}

	return nil
}

// ScopeHash computes a SHA-256 hash of the canonical JSON representation
// of the scopes for inclusion in JWT claims.
func ScopeHash(scopes []TokenScope) (string, error) {
	canonical, err := json.Marshal(scopes)
	if err != nil {
		return "", fmt.Errorf("marshaling scopes: %w", err)
	}
	hash := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", hash), nil
}
