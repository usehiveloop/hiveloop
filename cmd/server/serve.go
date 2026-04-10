package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/bootstrap"
	"github.com/ziraloop/ziraloop/internal/email"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/goroutine"
	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/hindsight"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/proxy"
)

func runServe(ctx context.Context, deps *bootstrap.Deps, enqueuer enqueue.TaskEnqueuer) error {
	cfg := deps.Config
	database := deps.DB
	redisClient := deps.Redis
	cacheManager := deps.CacheManager
	apiKeyCache := deps.APIKeyCache
	ctr := deps.Counter
	signingKey := deps.SigningKey
	rsaKey := deps.RSAKey
	reg := deps.Registry
	nangoClient := deps.NangoClient
	actionsCatalog := deps.ActionsCatalog
	sandboxEncKey := deps.SandboxEncKey
	orchestrator := deps.Orchestrator
	agentPusher := deps.AgentPusher
	eventBus := deps.EventBus

	logger := slog.Default()

	// Start cache invalidation subscriber (per-instance, real-time pub/sub)
	goroutine.Go(func() {
		if err := cacheManager.Invalidator().Subscribe(ctx); err != nil {
			slog.Error("invalidation subscriber stopped", "error", err)
		}
	})

	// Audit writer (buffered, non-blocking)
	auditWriter := middleware.NewAuditWriter(database, 10000)

	// Generation writer (buffered, non-blocking)
	generationWriter := middleware.NewGenerationWriter(database, reg, 10000)

	// Handlers
	mcpHandler := handler.NewMCPHandler(database, signingKey, actionsCatalog, nangoClient, ctr)
	credHandler := handler.NewCredentialHandler(database, deps.KMS, cacheManager, ctr)
	tokenHandler := handler.NewTokenHandler(database, signingKey, cacheManager, ctr, actionsCatalog, cfg.MCPBaseURL, mcpHandler.ServerCache)
	identityHandler := handler.NewIdentityHandler(database, sandboxEncKey)
	providerHandler := handler.NewProviderHandler(reg)
	connectSessionHandler := handler.NewConnectSessionHandler(database)
	connectAPIHandler := handler.NewConnectAPIHandler(database, deps.KMS, reg, nangoClient, actionsCatalog)
	settingsHandler := handler.NewSettingsHandler(database, sandboxEncKey)
	customDomainHandler := handler.NewCustomDomainHandler(database, cfg)
	integrationHandler := handler.NewIntegrationHandler(database, nangoClient, actionsCatalog)
	connectionHandler := handler.NewConnectionHandler(database, nangoClient, actionsCatalog)
	inIntegrationHandler := handler.NewInIntegrationHandler(database, nangoClient, actionsCatalog)
	inConnectionHandler := handler.NewInConnectionHandler(database, nangoClient, actionsCatalog)
	orgHandler := handler.NewOrgHandler(database)
	agentTriggerHandler := handler.NewAgentTriggerHandler(database, actionsCatalog)
	var emailSender email.Sender = &email.LogSender{}
	if enqueuer != nil {
		emailSender = email.NewAsynqSender(enqueuer)
	}
	authHandler := handler.NewAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		emailSender, cfg.FrontendURL, cfg.AutoConfirmEmail)
	if cfg.PlatformAdminEmails != "" {
		authHandler.SetPlatformAdminEmails(strings.Split(cfg.PlatformAdminEmails, ","))
	}
	if cfg.AdminAPIEnabled && cfg.PlatformAdminEmails != "" {
		authHandler.SetAdminMode(strings.Split(cfg.PlatformAdminEmails, ","))
	}
	authHandler.StartCleanup(ctx)
	oauthHandler := handler.NewOAuthHandler(database, rsaKey, signingKey,
		cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL,
		cfg.FrontendURL,
		cfg.OAuthGitHubClientID, cfg.OAuthGitHubClientSecret,
		cfg.OAuthGoogleClientID, cfg.OAuthGoogleClientSecret,
		cfg.OAuthXClientID, cfg.OAuthXClientSecret)
	apiKeyHandler := handler.NewAPIKeyHandler(database, apiKeyCache, cacheManager)
	usageHandler := handler.NewUsageHandler(database)
	auditHandler := handler.NewAuditHandler(database)
	generationHandler := handler.NewGenerationHandler(database)
	reportingHandler := handler.NewReportingHandler(database)
	proxyHandler := handler.NewProxyHandler(cacheManager, &proxy.CaptureTransport{Inner: proxy.NewTransport()})

	// Conversation + Forge handlers (require sandbox orchestrator)
	var conversationHandler *handler.ConversationHandler
	var forgeHandler *handler.ForgeHandler
	var systemConvHandler *handler.SystemConversationHandler
	forgeMCPHandler := forge.NewForgeMCPHandler(database)
	var forgeCtrl *forge.ForgeController
	if orchestrator != nil && agentPusher != nil {
		conversationHandler = handler.NewConversationHandler(database, orchestrator, agentPusher, eventBus)
		if enqueuer != nil {
			conversationHandler.SetEnqueuer(enqueuer)
		}
		systemConvHandler = handler.NewSystemConversationHandler(database, orchestrator, agentPusher, eventBus, signingKey, cfg)
		forgeCtrl = forge.NewForgeController(database, orchestrator, agentPusher, signingKey, cfg, eventBus, catalog.Global(), reg, cfg.AsynqRedisOpt())
		forgeHandler = handler.NewForgeHandler(database, forgeCtrl, eventBus, enqueuer)
		slog.Info("forge controller ready")
	}

	bridgeWebhookHandler := handler.NewBridgeWebhookHandler(database, sandboxEncKey, eventBus)
	nangoWebhookHandler := handler.NewNangoWebhookHandler(database, cfg.NangoSecretKey, sandboxEncKey, enqueuer)

	var templateBuilder handler.TemplateBuildable
	if orchestrator != nil {
		templateBuilder = orchestrator
	}
	sandboxTemplateHandler := handler.NewSandboxTemplateHandler(database, templateBuilder, enqueuer)
	skillHandler := handler.NewSkillHandler(database, enqueuer)

	var pusherForHandler handler.AgentPusher
	if agentPusher != nil {
		pusherForHandler = agentPusher
	}
	agentHandler := handler.NewAgentHandler(database, reg, pusherForHandler, sandboxEncKey, enqueuer)
	agentHandler.SetCatalog(actionsCatalog)
	if forgeCtrl != nil {
		agentHandler.SetForgeController(forgeCtrl)
	}
	marketplaceHandler := handler.NewMarketplaceHandler(database, redisClient)

	// Drive handler (optional — nil S3Client means drive is disabled)
	var driveHandler *handler.DriveHandler
	if deps.S3Client != nil {
		driveHandler = handler.NewDriveHandler(database, deps.S3Client)
	}

	// Billing handler (optional — nil PolarClient means billing is disabled)
	var billingHandler *handler.BillingHandler
	if deps.PolarClient != nil {
		billingHandler = handler.NewBillingHandler(database, deps.PolarClient, cfg)
	}

	// Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RequestLog(logger))

	// Health checks
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(database, redisClient))

	// Provider discovery (no auth)
	r.Get("/v1/providers", providerHandler.List)
	r.Get("/v1/providers/{id}", providerHandler.Get)
	r.Get("/v1/providers/{id}/models", providerHandler.Models)

	// In-integration discovery (no auth)
	r.Get("/v1/in/integrations/available", inIntegrationHandler.ListAvailable)

	// Integration catalog discovery (no auth)
	actionsHandler := handler.NewActionsHandler(actionsCatalog)
	r.Get("/v1/catalog/integrations", actionsHandler.ListIntegrations)
	r.Get("/v1/catalog/integrations/{id}", actionsHandler.GetIntegration)
	r.Get("/v1/catalog/integrations/{id}/actions", actionsHandler.ListActions)
	r.Get("/v1/catalog/integrations/{id}/triggers", actionsHandler.ListTriggers)
	r.Get("/v1/catalog/integrations/{id}/schema-paths", actionsHandler.GetSchemaPaths)

	// Marketplace discovery (no auth, Redis cached)
	r.Get("/v1/marketplace/agents", marketplaceHandler.List)
	r.Get("/v1/marketplace/agents/{slug}", marketplaceHandler.GetBySlug)

	// Webhook receivers (HMAC-verified, no auth middleware)
	r.Post("/internal/webhooks/bridge/{sandboxID}", bridgeWebhookHandler.Handle)
	r.Post("/internal/webhooks/nango", nangoWebhookHandler.Handle)
	if cfg.PolarWebhookSecret != "" {
		polarWebhookHandler := handler.NewPolarWebhookHandler(database, cfg.PolarWebhookSecret, cfg.PolarProductFreeID)
		r.Post("/internal/webhooks/polar", polarWebhookHandler.Handle)
	}

	// Embedded auth
	rsaPub := rsaKey.Public().(*rsa.PublicKey)

	// Auth routes
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/otp/request", authHandler.OTPRequest)
		r.Post("/otp/verify", authHandler.OTPVerify)
		r.Post("/confirm-email", authHandler.ConfirmEmail)
		r.Post("/resend-confirmation", authHandler.ResendConfirmation)
		r.Post("/forgot-password", authHandler.ForgotPassword)
		r.Post("/reset-password", authHandler.ResetPassword)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
			r.Post("/logout", authHandler.Logout)
			r.Get("/me", authHandler.Me)
			r.Post("/change-password", authHandler.ChangePassword)
		})
	})

	// OAuth social login
	r.Route("/oauth", func(r chi.Router) {
		r.Use(middleware.AuthRateLimit(ctx, 10, 20))
		r.Get("/github", oauthHandler.GitHubLogin)
		r.Get("/github/callback", oauthHandler.GitHubCallback)
		r.Get("/google", oauthHandler.GoogleLogin)
		r.Get("/google/callback", oauthHandler.GoogleCallback)
		r.Get("/x", oauthHandler.XLogin)
		r.Get("/x/callback", oauthHandler.XCallback)
		r.Post("/exchange", oauthHandler.Exchange)
	})

	// Org-authenticated routes
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.MultiAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience, database, apiKeyCache, enqueuer))
		r.Use(middleware.RequireEmailConfirmed(database))

		r.Post("/orgs", orgHandler.Create)

		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Use(middleware.RateLimit())
			r.Use(middleware.Audit(auditWriter))

			r.Get("/orgs/current", orgHandler.Current)
			r.Get("/usage", usageHandler.Get)
			r.Get("/audit", auditHandler.List)
			r.Get("/reporting", reportingHandler.Get)
			r.Get("/generations", generationHandler.List)
			r.Get("/generations/{id}", generationHandler.Get)

			r.Post("/api-keys", apiKeyHandler.Create)
			r.Get("/api-keys", apiKeyHandler.List)
			r.Delete("/api-keys/{id}", apiKeyHandler.Revoke)

			if billingHandler != nil {
				r.Post("/billing/checkout", billingHandler.CreateCheckout)
				r.Get("/billing/subscription", billingHandler.GetSubscription)
				r.Post("/billing/portal", billingHandler.CreatePortal)
			}

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("credentials"))
				r.Post("/credentials", credHandler.Create)
				r.Get("/credentials", credHandler.List)
				r.Get("/credentials/{id}", credHandler.Get)
				r.Delete("/credentials/{id}", credHandler.Revoke)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("tokens"))
				r.Get("/tokens", tokenHandler.List)
				r.Post("/tokens", tokenHandler.Mint)
				r.Delete("/tokens/{jti}", tokenHandler.Revoke)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Post("/identities", identityHandler.Create)
				r.Get("/identities", identityHandler.List)
				r.Get("/identities/{id}", identityHandler.Get)
				r.Put("/identities/{id}", identityHandler.Update)
				r.Delete("/identities/{id}", identityHandler.Delete)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("connect"))
				r.Post("/connect/sessions", connectSessionHandler.Create)
				r.Get("/connect/sessions", connectSessionHandler.List)
				r.Get("/connect/sessions/{id}", connectSessionHandler.Get)
				r.Delete("/connect/sessions/{id}", connectSessionHandler.Delete)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
				r.Get("/integrations/providers", integrationHandler.ListProviders)
				r.Post("/integrations", integrationHandler.Create)
				r.Get("/integrations", integrationHandler.List)
				r.Get("/integrations/{id}", integrationHandler.Get)
				r.Put("/integrations/{id}", integrationHandler.Update)
				r.Delete("/integrations/{id}", integrationHandler.Delete)
				r.Post("/integrations/{id}/connections", connectionHandler.Create)
				r.Get("/integrations/{id}/connections", connectionHandler.List)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
				r.Get("/connections/available-scopes", connectionHandler.AvailableScopes)
				r.Get("/connections/{id}", connectionHandler.Get)
				r.HandleFunc("/connections/{id}/proxy/*", connectionHandler.Proxy)
				r.Delete("/connections/{id}", connectionHandler.Revoke)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
				r.Route("/sandbox-templates", func(r chi.Router) {
					r.Post("/", sandboxTemplateHandler.Create)
					r.Get("/", sandboxTemplateHandler.List)
					r.Get("/{id}", sandboxTemplateHandler.Get)
					r.Put("/{id}", sandboxTemplateHandler.Update)
					r.Delete("/{id}", sandboxTemplateHandler.Delete)
					r.Post("/{id}/build", sandboxTemplateHandler.TriggerBuild)
					r.Post("/{id}/retry", sandboxTemplateHandler.RetryBuild)
				})
				r.Route("/skills", func(r chi.Router) {
					r.Post("/", skillHandler.Create)
					r.Get("/", skillHandler.List)
					r.Get("/{id}", skillHandler.Get)
					r.Patch("/{id}", skillHandler.Update)
					r.Delete("/{id}", skillHandler.Delete)
					r.Post("/{id}/hydrate", skillHandler.Hydrate)
					r.Get("/{id}/versions", skillHandler.ListVersions)
				})
				r.Route("/agents", func(r chi.Router) {
					r.Post("/", agentHandler.Create)
					r.Get("/", agentHandler.List)
					r.Get("/{id}", agentHandler.Get)
					r.Put("/{id}", agentHandler.Update)
					r.Delete("/{id}", agentHandler.Delete)
					r.Get("/{id}/setup", agentHandler.GetSetup)
					r.Put("/{id}/setup", agentHandler.UpdateSetup)
					if conversationHandler != nil {
						r.Post("/{agentID}/conversations", conversationHandler.Create)
						r.Get("/{agentID}/conversations", conversationHandler.List)
					}
					if forgeHandler != nil {
						r.Post("/{agentID}/forge", forgeHandler.Start)
						r.Get("/{agentID}/forge", forgeHandler.GetLatestRun)
					}
					r.Route("/{agentID}/triggers", func(r chi.Router) {
						r.Post("/", agentTriggerHandler.Create)
						r.Get("/", agentTriggerHandler.List)
						r.Get("/{id}", agentTriggerHandler.Get)
						r.Put("/{id}", agentTriggerHandler.Update)
						r.Delete("/{id}", agentTriggerHandler.Delete)
					})
					r.Route("/{agentID}/skills", func(r chi.Router) {
						r.Post("/", skillHandler.AttachToAgent)
						r.Get("/", skillHandler.ListAgentSkills)
						r.Delete("/{skillID}", skillHandler.DetachFromAgent)
					})
				})
				r.Route("/marketplace/agents", func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					r.Post("/", marketplaceHandler.Create)
					r.Put("/{id}", marketplaceHandler.Update)
					r.Delete("/{id}", marketplaceHandler.Delete)
				})
				if forgeHandler != nil {
					r.Route("/forge-runs/{runID}", func(r chi.Router) {
						r.Get("/", forgeHandler.GetRun)
						r.Get("/stream", forgeHandler.Stream)
						r.Get("/events", forgeHandler.ListEvents)
						r.Post("/cancel", forgeHandler.Cancel)
						r.Post("/apply", forgeHandler.Apply)
						r.Get("/iterations/{iterationID}/evals", forgeHandler.ListEvals)
						r.Get("/eval-cases", forgeHandler.ListEvalCases)
						r.Post("/eval-cases", forgeHandler.CreateEvalCase)
						r.Get("/eval-cases/{caseID}", forgeHandler.GetEvalCase)
						r.Put("/eval-cases/{caseID}", forgeHandler.UpdateEvalCase)
						r.Delete("/eval-cases/{caseID}", forgeHandler.DeleteEvalCase)
						r.Post("/approve-context", forgeHandler.ApproveContext)
						r.Post("/approve-evals", forgeHandler.ApproveEvals)
					})
				}
				if conversationHandler != nil {
					r.Route("/conversations/{convID}", func(r chi.Router) {
						r.Get("/", conversationHandler.Get)
						r.Delete("/", conversationHandler.End)
						r.Post("/messages", conversationHandler.SendMessage)
						r.Get("/stream", conversationHandler.Stream)
						r.Post("/abort", conversationHandler.Abort)
						r.Get("/approvals", conversationHandler.ListApprovals)
						r.Post("/approvals/{requestID}", conversationHandler.ResolveApproval)
						r.Get("/events", conversationHandler.ListEvents)
					})
				}
				if systemConvHandler != nil {
					r.Post("/system-agents/{type}/conversations", systemConvHandler.Create)
				}
				r.Route("/sandboxes", func(r chi.Router) {
					sandboxHandler := handler.NewSandboxHandler(database, orchestrator)
					r.Get("/", sandboxHandler.List)
					r.Get("/{id}", sandboxHandler.Get)
					if orchestrator != nil {
						r.Post("/{id}/stop", sandboxHandler.Stop)
						r.Post("/{id}/exec", sandboxHandler.Exec)
						r.Delete("/{id}", sandboxHandler.Delete)
					}
				})
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Get("/settings/connect", settingsHandler.GetConnectSettings)
				r.Put("/settings/connect", settingsHandler.UpdateConnectSettings)
				r.Get("/settings/webhooks", settingsHandler.GetWebhookSettings)
				r.Put("/settings/webhooks", settingsHandler.UpdateWebhookSettings)
				r.Post("/settings/webhooks/rotate-secret", settingsHandler.RotateWebhookSecret)
				r.Delete("/settings/webhooks", settingsHandler.DeleteWebhookSettings)
				r.Post("/custom-domains", customDomainHandler.Create)
				r.Get("/custom-domains", customDomainHandler.List)
				r.Post("/custom-domains/{id}/verify", customDomainHandler.Verify)
				r.Delete("/custom-domains/{id}", customDomainHandler.Delete)
			})
		})
	})

	// Connect API (session-authenticated)
	r.Route("/v1/widget", func(r chi.Router) {
		r.Use(middleware.ConnectSessionAuth(database))
		r.Use(middleware.ConnectSecurityHeaders())
		r.Use(middleware.ConnectCORS())
		r.Get("/session", connectAPIHandler.SessionInfo)
		r.Get("/providers", connectAPIHandler.ListProviders)
		r.Route("/integrations", func(r chi.Router) {
			r.Get("/providers", integrationHandler.ListProviders)
			r.Get("/", connectAPIHandler.ListIntegrations)
			r.Post("/{id}/connect-session", connectAPIHandler.CreateIntegrationConnectSession)
			r.Get("/{id}/resources/{type}/available", connectAPIHandler.ListAvailableResources)
			r.Post("/{id}/connections", connectAPIHandler.CreateIntegrationConnection)
			r.Patch("/{id}/connections/{connectionId}", connectAPIHandler.PatchIntegrationConnection)
			r.Delete("/{id}/connections/{connectionId}", connectAPIHandler.DeleteIntegrationConnection)
		})
		r.Get("/connections", connectAPIHandler.ListConnections)
		r.Post("/connections", connectAPIHandler.CreateConnection)
		r.Delete("/connections/{id}", connectAPIHandler.DeleteConnection)
		r.Post("/connections/{id}/verify", connectAPIHandler.VerifyConnection)
	})

	// In-integrations & in-connections
	var platformAdminEmails []string
	if cfg.PlatformAdminEmails != "" {
		platformAdminEmails = strings.Split(cfg.PlatformAdminEmails, ",")
	}
	r.Route("/v1/in", func(r chi.Router) {
		r.Use(middleware.RequireAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience))
		r.Use(middleware.RequireEmailConfirmed(database))
		r.Use(middleware.ResolveUser(database))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePlatformAdmin(platformAdminEmails))
			r.Post("/integrations", inIntegrationHandler.Create)
			r.Get("/integrations", inIntegrationHandler.List)
			r.Get("/integrations/{id}", inIntegrationHandler.Get)
			r.Put("/integrations/{id}", inIntegrationHandler.Update)
			r.Delete("/integrations/{id}", inIntegrationHandler.Delete)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Post("/integrations/{id}/connect-session", inConnectionHandler.CreateConnectSession)
			r.Post("/integrations/{id}/connections", inConnectionHandler.Create)
			r.Get("/connections", inConnectionHandler.List)
			r.Get("/connections/{id}", inConnectionHandler.Get)
			r.Delete("/connections/{id}", inConnectionHandler.Revoke)
		})
	})

	// Admin API
	if cfg.AdminAPIEnabled {
		adminHandler := handler.NewAdminHandler(database, orchestrator, nangoClient, actionsCatalog,
			rsaKey, signingKey, cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthAccessTokenTTL, cfg.AuthRefreshTokenTTL)
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
			r.Get("/identities", adminHandler.ListIdentities)
			r.Get("/identities/{id}", adminHandler.GetIdentity)
			r.Put("/identities/{id}", adminHandler.UpdateIdentity)
			r.Delete("/identities/{id}", adminHandler.DeleteIdentity)
			r.Get("/agents", adminHandler.ListAgents)
			r.Get("/agents/{id}", adminHandler.GetAgent)
			r.Put("/agents/{id}", adminHandler.UpdateAgent)
			r.Post("/agents/{id}/archive", adminHandler.ArchiveAgent)
			r.Delete("/agents/{id}", adminHandler.DeleteAgent)
			r.Get("/sandboxes", adminHandler.ListSandboxes)
			r.Get("/sandboxes/{id}", adminHandler.GetSandbox)
			r.Post("/sandboxes/{id}/stop", adminHandler.StopSandbox)
			r.Delete("/sandboxes/{id}", adminHandler.DeleteSandbox)
			r.Post("/sandboxes/cleanup", adminHandler.CleanupSandboxes)
			r.Get("/sandbox-templates", adminHandler.ListSandboxTemplates)
			r.Put("/sandbox-templates/{id}", adminHandler.UpdateSandboxTemplate)
			r.Delete("/sandbox-templates/{id}", adminHandler.DeleteSandboxTemplate)
			r.Get("/conversations", adminHandler.ListConversations)
			r.Get("/conversations/{id}", adminHandler.GetConversation)
			r.Delete("/conversations/{id}", adminHandler.EndConversation)
			r.Get("/forge-runs", adminHandler.ListForgeRuns)
			r.Get("/forge-runs/{id}", adminHandler.GetForgeRun)
			r.Post("/forge-runs/{id}/cancel", adminHandler.CancelForgeRun)
			r.Get("/generations", adminHandler.ListGenerations)
			r.Get("/generations/stats", adminHandler.GenerationStats)
			r.Get("/integrations", adminHandler.ListIntegrations)
			r.Get("/connections", adminHandler.ListConnections)
			r.Post("/connections/{id}/revoke", adminHandler.RevokeConnection)
			r.Get("/in-integration-providers", adminHandler.ListInIntegrationProviders)
			r.Post("/in-integrations", adminHandler.CreateInIntegration)
			r.Get("/in-integrations", adminHandler.ListInIntegrations)
			r.Get("/in-integrations/{id}", adminHandler.GetInIntegration)
			r.Put("/in-integrations/{id}", adminHandler.UpdateInIntegration)
			r.Delete("/in-integrations/{id}", adminHandler.DeleteInIntegration)
			r.Get("/in-connections", adminHandler.ListInConnections)
			r.Get("/connect-sessions", adminHandler.ListConnectSessions)
			r.Delete("/connect-sessions/{id}", adminHandler.DeleteConnectSession)
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

	// Proxy routes
	r.Route("/v1/proxy", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(middleware.IdentityRateLimit(redisClient, database))
		r.Use(middleware.RemainingCheck(ctr))
		r.Use(middleware.Audit(auditWriter, "proxy.request"))
		r.Use(middleware.Generation(generationWriter, database))
		r.Handle("/*", proxyHandler)
	})

	// Drive routes (agent file storage, authenticated with proxy token)
	if driveHandler != nil {
		r.Route("/v1/drive", func(r chi.Router) {
			r.Use(middleware.TokenAuth(signingKey, database))
			r.Post("/assets", driveHandler.Upload)
			r.Get("/assets", driveHandler.List)
			r.Get("/assets/{assetID}", driveHandler.Get)
			r.Delete("/assets/{assetID}", driveHandler.Delete)
		})
	}

	// Spider routes (web crawling/search via spider.cloud)
	// Mounted at /spider (not /v1/spider) to avoid the /v1 org-auth middleware group.
	// TODO: re-enable TokenAuth after verifying spider responses
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

	// Main HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	goroutine.Go(func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	})

	// MCP Server (separate port)
	mcpRouter := chi.NewRouter()
	mcpRouter.Use(chimw.RequestID)
	mcpRouter.Use(chimw.RealIP)
	mcpRouter.Use(chimw.Recoverer)
	mcpRouter.Use(middleware.RequestLog(logger))

	if cfg.HindsightAPIURL != "" {
		memoryHandler := hindsight.NewMemoryMCPHandler(database, hindsight.NewClient(cfg.HindsightAPIURL))
		mcpRouter.Route("/memory/{agentID}", func(r chi.Router) {
			r.Use(middleware.TokenAuth(signingKey, database))
			r.Use(memoryHandler.ValidateAgentToken)
			r.Handle("/*", memoryHandler.StreamableHTTPHandler())
			r.Handle("/", memoryHandler.StreamableHTTPHandler())
		})
		memoryHandler.StartCleanup(ctx, 5*time.Minute)
		slog.Info("hindsight memory MCP tools registered on /memory/{agentID}")
	}

	mcpRouter.Route("/forge/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeMCPHandler.StreamableHTTPHandler())
		r.Handle("/", forgeMCPHandler.StreamableHTTPHandler())
	})
	slog.Info("forge MCP tools registered on /forge/{forgeRunID}")

	forgeContextMCPHandler := forge.NewForgeContextMCPHandler(database)
	mcpRouter.Route("/forge-context/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeContextMCPHandler.StreamableHTTPHandler())
		r.Handle("/", forgeContextMCPHandler.StreamableHTTPHandler())
	})
	slog.Info("forge context MCP registered on /forge-context/{forgeRunID}")

	forgeArchitectMCP := forge.NewForgeArchitectMCPHandler(database)
	mcpRouter.Route("/forge-architect/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeArchitectMCP.StreamableHTTPHandler())
		r.Handle("/", forgeArchitectMCP.StreamableHTTPHandler())
	})

	forgeEvalDesignerMCP := forge.NewForgeEvalDesignerMCPHandler(database, eventBus).WithCatalog(catalog.Global())
	mcpRouter.Route("/forge-eval-designer/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeEvalDesignerMCP.StreamableHTTPHandler())
		r.Handle("/", forgeEvalDesignerMCP.StreamableHTTPHandler())
	})

	forgeJudgeMCP := forge.NewForgeJudgeMCPHandler(database)
	mcpRouter.Route("/forge-judge/{forgeRunID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", forgeJudgeMCP.StreamableHTTPHandler())
		r.Handle("/", forgeJudgeMCP.StreamableHTTPHandler())
	})
	slog.Info("forge agent MCP tools registered (architect, eval-designer, judge)")

	mcpRouter.Route("/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(mcpHandler.ValidateJTIMatch)
		r.Use(mcpHandler.ValidateHasScopes)
		r.Handle("/*", mcpHandler.StreamableHTTPHandler())
	})

	mcpRouter.Route("/sse/{jti}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Use(mcpHandler.ValidateJTIMatch)
		r.Use(mcpHandler.ValidateHasScopes)
		r.Handle("/*", mcpHandler.SSEHandler())
	})

	mcpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.MCPPort),
		Handler:      mcpRouter,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	mcpHandler.ServerCache.StartCleanup(ctx, 5*time.Minute)

	goroutine.Go(func() {
		slog.Info("mcp server starting", "port", cfg.MCPPort)
		if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server error", "error", err)
		}
	})

	// Wait for shutdown
	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := mcpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("mcp server shutdown error", "error", err)
	}

	auditWriter.Shutdown(shutdownCtx)
	generationWriter.Shutdown(shutdownCtx)
	if deps.ToolUsageWriter != nil {
		deps.ToolUsageWriter.Shutdown(shutdownCtx)
	}

	slog.Info("serve shutdown complete")
	return nil
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyz(database *gorm.DB, rc *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := database.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db connection failed"}`))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db ping failed"}`))
			return
		}

		if err := rc.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
