package main

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func setupProxyAndAuxRoutes(
	r chi.Router,
	cfg *config.Config,
	deps *bootstrap.Deps,
	signingKey []byte,
	database *gorm.DB,
	proxyHandler http.Handler,
	driveHandler *handler.DriveHandler,
	sandboxEncKey *crypto.SymmetricKey,
	auditWriter *middleware.AuditWriter,
	generationWriter *middleware.GenerationWriter,
	ctr *counter.Counter,
) {
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(middleware.RequireCredits(deps.Credits))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter, "proxy.request"))
		r.Use(middleware.Generation(generationWriter, database))
		r.Handle("/*", proxyHandler)
	})

	if driveHandler != nil {
		r.Route("/v1/drive", func(r chi.Router) {
			r.Use(middleware.TokenAuth(signingKey, database))
			r.Post("/assets", driveHandler.Upload)
			r.Get("/assets", driveHandler.List)
			r.Get("/assets/{assetID}", driveHandler.Get)
			r.Delete("/assets/{assetID}", driveHandler.Delete)
		})
	}

	if deps.S3Client != nil && sandboxEncKey != nil {
		sandboxDriveHandler := handler.NewSandboxDriveHandler(database, deps.S3Client, sandboxEncKey)
		r.Route("/internal/sandbox-drive/{sandboxID}", func(r chi.Router) {
			r.Post("/assets", sandboxDriveHandler.Upload)
			r.Get("/assets", sandboxDriveHandler.List)
			r.Get("/assets/{assetID}", sandboxDriveHandler.Get)
			r.Delete("/assets/{assetID}", sandboxDriveHandler.Delete)
		})
	}

	if deps.SpiderClient != nil {
		spiderHandler := handler.NewSpiderHandler(deps.SpiderClient, deps.ToolUsageWriter, database)
		r.Route("/spider", func(r chi.Router) {
			r.Post("/crawl", spiderHandler.Crawl)
			r.Post("/search", spiderHandler.Search)
			r.Post("/links", spiderHandler.Links)
			r.Post("/screenshot", spiderHandler.Screenshot)
			r.Post("/transform", spiderHandler.Transform)
		})
		slog.Warn("spider routes registered without authentication", "path", "/spider", "note", "temporary")
	}
}
