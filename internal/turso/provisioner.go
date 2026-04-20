package turso

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Provisioner manages per-workspace Turso database provisioning.
type Provisioner struct {
	client *Client
	group  string
	db     *gorm.DB
}

// NewProvisioner creates a workspace storage provisioner.
func NewProvisioner(client *Client, group string, db *gorm.DB) *Provisioner {
	return &Provisioner{
		client: client,
		group:  group,
		db:     db,
	}
}

// EnsureStorage provisions a Turso database for the workspace if one doesn't exist.
// Returns the storage URL and auth token needed for Bridge's env vars.
func (p *Provisioner) EnsureStorage(ctx context.Context, orgID uuid.UUID) (storageURL, authToken string, err error) {
	// Check if storage already exists for this org
	var existing model.WorkspaceStorage
	if err := p.db.Where("org_id = ?", orgID).First(&existing).Error; err == nil {
		return existing.StorageURL, existing.StorageAuthToken, nil
	}

	// Create a new Turso database
	dbName := "zira-" + shortID(orgID)
	database, err := p.client.CreateDatabase(ctx, dbName, p.group)
	if err != nil {
		return "", "", fmt.Errorf("provisioning turso database: %w", err)
	}

	// Mint an auth token
	token, err := p.client.CreateToken(ctx, dbName)
	if err != nil {
		return "", "", fmt.Errorf("minting turso token: %w", err)
	}

	storageURL = DatabaseURL(database.Hostname)

	// Store in DB
	ws := model.WorkspaceStorage{
		OrgID:             orgID,
		TursoDatabaseName: dbName,
		StorageURL:        storageURL,
		StorageAuthToken:  token,
	}
	if err := p.db.Create(&ws).Error; err != nil {
		return "", "", fmt.Errorf("saving workspace storage: %w", err)
	}

	return storageURL, token, nil
}

// DeleteStorage removes the Turso database for a workspace.
func (p *Provisioner) DeleteStorage(ctx context.Context, orgID uuid.UUID) error {
	var ws model.WorkspaceStorage
	if err := p.db.Where("org_id = ?", orgID).First(&ws).Error; err != nil {
		return nil // nothing to delete
	}

	if err := p.client.DeleteDatabase(ctx, ws.TursoDatabaseName); err != nil {
		return fmt.Errorf("deleting turso database: %w", err)
	}

	return p.db.Delete(&ws).Error
}

// shortID returns the first 12 chars of a UUID (without hyphens) for database naming.
func shortID(id uuid.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")[:12]
}
