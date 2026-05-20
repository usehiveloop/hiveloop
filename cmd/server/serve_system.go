package main

import (
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/system"
	// Blank import: each task file's init() registers itself in
	// system.registry.
	_ "github.com/usehivy/hivy/internal/system/tasks"
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
