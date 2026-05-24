package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/nango"
)

const managedBy = "global_integrations"

type SeedResult struct {
	Created   int
	Updated   int
	Unchanged int
	Deleted   int
	Skipped   int
}

type Seeder struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

func NewSeeder(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *Seeder {
	if cat == nil {
		cat = catalog.Global()
	}
	return &Seeder{db: db, nango: nangoClient, catalog: cat}
}

func SeedGlobalIntegrations(ctx context.Context, db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog, dir string) (*SeedResult, error) {
	return NewSeeder(db, nangoClient, cat).Seed(ctx, dir)
}

func (s *Seeder) Seed(ctx context.Context, dir string) (*SeedResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if s.nango == nil {
		return nil, fmt.Errorf("nango client is required")
	}
	manifests, err := loadManifests(dir)
	if err != nil {
		return nil, err
	}
	if err := validateManifests(manifests); err != nil {
		return nil, err
	}
	result := &SeedResult{}
	seen := map[string]bool{}
	for _, manifest := range manifests {
		seen[manifest.ID] = true
		state, err := s.syncOne(ctx, manifest)
		if err != nil {
			return result, err
		}
		switch state {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "unchanged":
			result.Unchanged++
		case "deleted":
			result.Deleted++
		case "skipped":
			result.Skipped++
		}
	}
	deleted, err := s.disableMissing(ctx, seen)
	if err != nil {
		return result, err
	}
	result.Deleted += deleted
	return result, nil
}

func validateManifests(manifests []Manifest) error {
	ids := map[string]string{}
	keys := map[string]string{}
	for _, m := range manifests {
		if err := validateManifest(m); err != nil {
			return err
		}
		if prior, ok := ids[m.ID]; ok {
			return fmt.Errorf("duplicate global integration id %q in %s and %s", m.ID, prior, m.SourcePath)
		}
		if prior, ok := keys[m.UniqueKey]; ok {
			return fmt.Errorf("duplicate global integration unique_key %q in %s and %s", m.UniqueKey, prior, m.SourcePath)
		}
		ids[m.ID] = m.SourcePath
		keys[m.UniqueKey] = m.SourcePath
	}
	return nil
}

func validateManifest(m Manifest) error {
	if m.Version != 1 {
		return fmt.Errorf("%s: unsupported version %d", m.SourcePath, m.Version)
	}
	for field, value := range map[string]string{
		"id":           m.ID,
		"provider":     m.Provider,
		"unique_key":   m.UniqueKey,
		"display_name": m.DisplayName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s: %s is required", m.SourcePath, field)
		}
	}
	return nil
}

func manifestHash(m Manifest) (string, error) {
	raw := map[string]interface{}{}
	for k, v := range m.raw {
		raw[k] = v
	}
	delete(raw, "source_path")
	body, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func enabled(m Manifest) bool {
	return m.Enabled == nil || *m.Enabled
}

func nangoKey(uniqueKey string) string {
	return uniqueKey
}

func nangoProvider(m Manifest) string {
	if strings.TrimSpace(m.NangoProvider) != "" {
		return strings.TrimSpace(m.NangoProvider)
	}
	return strings.TrimSpace(m.Provider)
}

func logSkip(ctx context.Context, m Manifest, reason string) {
	logging.FromContext(ctx).WarnContext(ctx, "global integration skipped",
		"id", m.ID, "provider", m.Provider, "reason", reason)
}

func isNotFound(err error) bool {
	return errors.Is(err, nango.ErrNotFound)
}

func nowPtr() *time.Time {
	now := time.Now()
	return &now
}
