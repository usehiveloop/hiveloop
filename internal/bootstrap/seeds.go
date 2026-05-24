package bootstrap

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/integrations"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/skills"
)

func seedGlobalSkills(ctx context.Context, database *gorm.DB) error {
	result, err := skills.SeedGlobalSkills(ctx, database, "global/skills")
	if err != nil {
		return fmt.Errorf("seeding global skills: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global skills seeded",
		"created", result.Created,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
	)
	return nil
}

func seedGlobalLLMCredentials(ctx context.Context, database *gorm.DB, kms *crypto.KeyWrapper, cacheManager *cache.Manager, ctr *counter.Counter) error {
	result, err := credentials.SeedGlobalLLMCredentials(ctx, database, kms, cacheManager, ctr, "global/credentials/llm.json")
	if err != nil {
		return fmt.Errorf("seeding global LLM credentials: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global LLM credentials seeded",
		"created", result.Created,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
		"revoked", result.Revoked,
		"skipped", result.Skipped,
	)
	return nil
}

func seedGlobalIntegrations(ctx context.Context, database *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) error {
	result, err := integrations.SeedGlobalIntegrations(ctx, database, nangoClient, cat, "global/integrations")
	if err != nil {
		return fmt.Errorf("seeding global integrations: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "global integrations seeded",
		"created", result.Created,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
		"deleted", result.Deleted,
		"skipped", result.Skipped,
	)
	return nil
}
