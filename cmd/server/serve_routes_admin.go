package main

import (
	"crypto/rsa"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
)

func setupAdminRoutes(
	r chi.Router,
	cfg *bootstrap.Config,
	deps *bootstrap.Deps,
	rsaPub *rsa.PublicKey,
	database *gorm.DB,
	platformAdminEmails []string,
	enqueuer enqueue.TaskEnqueuer,
	marketplaceHandler *handler.MarketplaceHandler,
) {
	if cfg.AdminAPIEnabled {
		adminHandler := handler.NewAdminHandler(database, deps.Orchestrator, deps.NangoClient, deps.ActionsCatalog,
			deps.RSAKey, deps.SigningKey, cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL, enqueuer)
		r.Route("/admin/v1", func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Use(middleware.RequireEmailConfirmed(database))
			r.Use(middleware.ResolveUser(database))
			r.Use(middleware.RequirePlatformAdmin(platformAdminEmails))
			r.Use(middleware.AdminAudit(database, enqueuer))

			r.Get("/stats", adminHandler.Stats)
			r.Get("/users", adminHandler.ListUsers)
			r.Get("/users/{id}", adminHandler.GetUser)
			r.Put("/users/{id}", adminHandler.UpdateUser)
			r.Post("/users/{id}/ban", adminHandler.BanUser)
			r.Post("/users/{id}/unban", adminHandler.UnbanUser)
			r.Post("/users/{id}/confirm-email", adminHandler.ConfirmUserEmail)
			r.Delete("/users/{id}", adminHandler.DeleteUser)
			r.Post("/users/{id}/impersonate", adminHandler.Impersonate)
			r.Get("/orgs", adminHandler.ListOrgs)
			r.Get("/orgs/{id}", adminHandler.GetOrg)
			r.Put("/orgs/{id}", adminHandler.UpdateOrgFull)
			r.Post("/orgs/{id}/deactivate", adminHandler.DeactivateOrg)
			r.Post("/orgs/{id}/activate", adminHandler.ActivateOrg)
			r.Get("/orgs/{id}/members", adminHandler.ListOrgMembers)
			r.Delete("/orgs/{id}", adminHandler.DeleteOrg)
			r.Get("/credentials", adminHandler.ListCredentials)
			r.Get("/credentials/{id}", adminHandler.GetCredential)
			r.Put("/credentials/{id}", adminHandler.UpdateCredential)
			r.Post("/credentials/{id}/revoke", adminHandler.RevokeCredential)
			r.Get("/api-keys", adminHandler.ListAPIKeys)
			r.Post("/api-keys/{id}/revoke", adminHandler.RevokeAPIKey)
			r.Get("/tokens", adminHandler.ListTokens)
			r.Post("/tokens/{id}/revoke", adminHandler.RevokeToken)
			r.Get("/agents", adminHandler.ListAgents)
			r.Get("/agents/{id}", adminHandler.GetAgent)
			r.Put("/agents/{id}", adminHandler.UpdateAgent)
			r.Post("/agents/{id}/archive", adminHandler.ArchiveAgent)
			r.Delete("/agents/{id}", adminHandler.DeleteAgent)
			r.Get("/skills", adminHandler.ListSkills)
			r.Get("/skills/{id}", adminHandler.GetSkill)
			r.Post("/skills", adminHandler.CreateSkill)
			r.Put("/skills/{id}", adminHandler.UpdateSkill)
			r.Delete("/skills/{id}", adminHandler.DeleteSkill)
			r.Get("/sandboxes", adminHandler.ListSandboxes)
			r.Get("/sandboxes/{id}", adminHandler.GetSandbox)
			r.Post("/sandboxes/{id}/stop", adminHandler.StopSandbox)
			r.Delete("/sandboxes/{id}", adminHandler.DeleteSandbox)
			r.Post("/sandboxes/cleanup", adminHandler.CleanupSandboxes)
			r.Get("/sandbox-templates", adminHandler.ListSandboxTemplates)
			r.Post("/sandbox-templates", adminHandler.CreateSandboxTemplate)
			r.Get("/sandbox-templates/{id}", adminHandler.GetSandboxTemplate)
			r.Put("/sandbox-templates/{id}", adminHandler.UpdateSandboxTemplate)
			r.Delete("/sandbox-templates/{id}", adminHandler.DeleteSandboxTemplate)
			r.Get("/conversations", adminHandler.ListConversations)
			r.Get("/conversations/{id}", adminHandler.GetConversation)
			r.Delete("/conversations/{id}", adminHandler.EndConversation)
			r.Get("/generations", adminHandler.ListGenerations)
			r.Get("/generations/stats", adminHandler.GenerationStats)
			r.Get("/in-integration-providers", adminHandler.ListInIntegrationProviders)
			r.Post("/in-integrations", adminHandler.CreateInIntegration)
			r.Get("/in-integrations", adminHandler.ListInIntegrations)
			r.Get("/in-integrations/{id}", adminHandler.GetInIntegration)
			r.Put("/in-integrations/{id}", adminHandler.UpdateInIntegration)
			r.Delete("/in-integrations/{id}", adminHandler.DeleteInIntegration)
			r.Get("/in-connections", adminHandler.ListInConnections)
			r.Get("/custom-domains", adminHandler.ListCustomDomains)
			r.Delete("/custom-domains/{id}", adminHandler.DeleteCustomDomain)
			r.Get("/audit", adminHandler.ListAudit)
			r.Get("/usage", adminHandler.ListUsage)
			r.Get("/admin-audit", adminHandler.ListAdminAudit)
			r.Get("/workspace-storage", adminHandler.ListWorkspaceStorage)
			r.Delete("/workspace-storage/{id}", adminHandler.DeleteWorkspaceStorage)
			r.Get("/marketplace/agents", marketplaceHandler.AdminList)
			r.Put("/marketplace/agents/{id}", marketplaceHandler.AdminUpdate)
			r.Delete("/marketplace/agents/{id}", marketplaceHandler.AdminDelete)
			r.Post("/marketplace/cache/bust", marketplaceHandler.BustCache)
		})
		slog.Info("admin API enabled", "path", "/admin/v1")
	}
}

func setupProxyAndAuxRoutes(
	r chi.Router,
	cfg *bootstrap.Config,
	deps *bootstrap.Deps,
	signingKey []byte,
	database *gorm.DB,
	proxyHandler *handler.ProxyHandler,
	driveHandler *handler.DriveHandler,
	sandboxEncKey []byte,
	auditWriter *middleware.AuditWriter,
	generationWriter *middleware.GenerationWriter,
	ctr bootstrap.Counter,
) {
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
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
		slog.Info("spider routes registered (NO AUTH - temporary)")
	}
}
