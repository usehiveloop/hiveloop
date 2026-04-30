package main

import (
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/system"
	// Blank import: each task file's init() registers itself in
	// system.registry.
	_ "github.com/usehiveloop/hiveloop/internal/system/tasks"
)

func buildSystemTaskHandler(db *gorm.DB, deps *bootstrap.Deps, redisClient *redis.Client) *handler.SystemTaskHandler {
	picker := credentials.NewPicker(db)
	httpClient := &http.Client{
		Transport: &proxy.CaptureTransport{Inner: proxy.NewTransport()},
	}
	fwd := system.NewForwarder(httpClient)
	cache := system.NewRedisCache(redisClient)
	return handler.NewSystemTaskHandler(
		db,
		picker,
		deps.KMS,
		registry.Global(),
		cache,
		fwd,
		deps.Credits,
		catalog.Global(),
	)
}
