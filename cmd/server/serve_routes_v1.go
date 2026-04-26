package main

import (
	"crypto/rsa"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func setupV1Routes(
	r chi.Router,
	cfg *config.Config,
	rsaPub *rsa.PublicKey,
	database *gorm.DB,
	apiKeyCache *cache.APIKeyCache,
	enqueuer enqueue.TaskEnqueuer,
	orgHandler *handler.OrgHandler,
	orgInviteHandler *handler.OrgInviteHandler,
	usageHandler *handler.UsageHandler,
	auditHandler *handler.AuditHandler,
	reportingHandler *handler.ReportingHandler,
	generationHandler *handler.GenerationHandler,
	apiKeyHandler *handler.APIKeyHandler,
	billingHandler *handler.BillingHandler,
	credHandler *handler.CredentialHandler,
	tokenHandler *handler.TokenHandler,
	sandboxTemplateHandler *handler.SandboxTemplateHandler,
	skillHandler *handler.SkillHandler,
	subagentHandler *handler.SubagentHandler,
	agentHandler *handler.AgentHandler,
	marketplaceHandler *handler.MarketplaceHandler,
	conversationHandler *handler.ConversationHandler,
	routerHandler *handler.RouterHandler,
	customDomainHandler *handler.CustomDomainHandler,
	ragSourceHandler *handler.RAGSourceHandler,
	uploadsHandler *handler.UploadsHandler,
	orchestrator *sandbox.Orchestrator,
	auditWriter *middleware.AuditWriter,
) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.MultiAuth(rsaPub, cfg.AuthIssuer, cfg.AuthAudience, database, apiKeyCache, enqueuer))
		r.Use(middleware.RequireEmailConfirmed(database))

		r.Post("/orgs", orgHandler.Create)

		// Authenticated invite accept/decline — user-scoped, no org context required.
		r.Post("/invites/{token}/accept", orgInviteHandler.Accept)
		r.Post("/invites/{token}/decline", orgInviteHandler.Decline)

		r.Group(func(r chi.Router) {
			r.Use(middleware.ResolveOrgFlexible(database))
			r.Use(middleware.RateLimit())
			r.Use(middleware.Audit(auditWriter))

			r.Get("/orgs/current", orgHandler.Current)
			r.Get("/orgs/current/members", orgInviteHandler.ListMembers)

			// Admin-only org invite management.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdmin(database))
				r.Post("/orgs/current/invites", orgInviteHandler.Create)
				r.Get("/orgs/current/invites", orgInviteHandler.List)
				r.Delete("/orgs/current/invites/{id}", orgInviteHandler.Revoke)
				r.Post("/orgs/current/invites/{id}/resend", orgInviteHandler.Resend)
			})
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
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("connect"))
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("integrations"))
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("agents"))
				r.Route("/sandbox-templates", func(r chi.Router) {
					r.Post("/", sandboxTemplateHandler.Create)
					r.Get("/", sandboxTemplateHandler.List)
					r.Get("/public", sandboxTemplateHandler.ListPublic)
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
					r.Put("/{id}/content", skillHandler.UpdateContent)
					r.Post("/{id}/hydrate", skillHandler.Hydrate)
					r.Get("/{id}/versions", skillHandler.ListVersions)
					r.Post("/{id}/publish", skillHandler.Publish)
					r.Delete("/{id}/publish", skillHandler.Unpublish)
				})
				r.Route("/subagents", func(r chi.Router) {
					r.Post("/", subagentHandler.Create)
					r.Get("/", subagentHandler.List)
					r.Get("/{id}", subagentHandler.Get)
					r.Patch("/{id}", subagentHandler.Update)
					r.Delete("/{id}", subagentHandler.Delete)
				})
				r.Get("/agents/sandbox-tools", agentHandler.ListSandboxTools)
				r.Get("/agents/built-in-tools", agentHandler.ListBuiltInTools)
				r.Get("/agents/categories", agentHandler.ListCategories)
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
					// Agent triggers removed — routing is now handled by the
					// Zira router at /v1/router/triggers.
				})

				// Zira Router — unified routing identity for the org.
				r.Route("/router", func(r chi.Router) {
					r.Get("/", routerHandler.GetOrCreateRouter)
					r.Put("/", routerHandler.UpdateRouter)
					r.Post("/triggers", routerHandler.CreateTrigger)
					r.Get("/triggers", routerHandler.ListTriggers)
					r.Delete("/triggers/{id}", routerHandler.DeleteTrigger)
					r.Post("/triggers/{id}/rules", routerHandler.CreateRule)
					r.Get("/triggers/{id}/rules", routerHandler.ListRules)
					r.Delete("/triggers/{id}/rules/{ruleID}", routerHandler.DeleteRule)
					r.Get("/decisions", routerHandler.ListDecisions)
					r.Route("/{agentID}/skills", func(r chi.Router) {
						r.Post("/", skillHandler.AttachToAgent)
						r.Get("/", skillHandler.ListAgentSkills)
						r.Delete("/{skillID}", skillHandler.DetachFromAgent)
					})
					r.Route("/{agentID}/subagents", func(r chi.Router) {
						r.Post("/", subagentHandler.AttachToAgent)
						r.Get("/", subagentHandler.ListAgentSubagents)
						r.Delete("/{subagentID}", subagentHandler.DetachFromAgent)
					})
				})
				r.Route("/marketplace/agents", func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					r.Post("/", marketplaceHandler.Create)
					r.Put("/{id}", marketplaceHandler.Update)
					r.Delete("/{id}", marketplaceHandler.Delete)
				})
				if conversationHandler != nil {
					r.Route("/conversations/{convID}", func(r chi.Router) {
						r.Get("/", conversationHandler.Get)
						r.Delete("/", conversationHandler.End)
						r.Post("/messages", conversationHandler.SendMessage)
						r.Get("/stream", conversationHandler.Stream)
						r.Get("/history", conversationHandler.History)
						r.Post("/abort", conversationHandler.Abort)
						r.Get("/approvals", conversationHandler.ListApprovals)
						r.Post("/approvals/{requestID}", conversationHandler.ResolveApproval)
						r.Get("/events", conversationHandler.ListEvents)
					})
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

			if ragSourceHandler != nil {
				r.Route("/rag", func(r chi.Router) {
					r.Use(middleware.RequireOrgAdmin(database))
					r.Get("/integrations", ragSourceHandler.ListIntegrations)
					r.Post("/sources", ragSourceHandler.Create)
					r.Get("/sources", ragSourceHandler.List)
					r.Get("/sources/{id}", ragSourceHandler.Get)
					r.Patch("/sources/{id}", ragSourceHandler.Update)
					r.Delete("/sources/{id}", ragSourceHandler.Delete)
					r.Post("/sources/{id}/sync", ragSourceHandler.TriggerSync)
					r.Post("/sources/{id}/prune", ragSourceHandler.TriggerPrune)
					r.Post("/sources/{id}/perm-sync", ragSourceHandler.TriggerPermSync)
					r.Get("/sources/{id}/attempts", ragSourceHandler.ListAttempts)
					r.Get("/sources/{id}/attempts/{attempt_id}", ragSourceHandler.GetAttempt)
				})
			}

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAPIKeyScopeOrJWT("all"))
				r.Post("/custom-domains", customDomainHandler.Create)
				r.Get("/custom-domains", customDomainHandler.List)
				r.Post("/custom-domains/{id}/verify", customDomainHandler.Verify)
				r.Delete("/custom-domains/{id}", customDomainHandler.Delete)
			})

			if uploadsHandler != nil {
				r.Route("/uploads", func(r chi.Router) {
					r.Use(middleware.ResolveUser(database))
					r.Post("/sign", uploadsHandler.Sign)
				})
			}
		})
	})
}

func setupConnectRoutes(
	r chi.Router,
	cfg *config.Config,
	rsaPub *rsa.PublicKey,
	database *gorm.DB,
	platformAdminEmails []string,
	inIntegrationHandler *handler.InIntegrationHandler,
	inConnectionHandler *handler.InConnectionHandler,
) {
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
			r.Get("/connections/{id}/resources/{type}", inConnectionHandler.ListResources)
			r.Post("/connections/{id}/reconnect-session", inConnectionHandler.CreateReconnectSession)
			r.Patch("/connections/{id}/webhook-configured", inConnectionHandler.MarkWebhookConfigured)
			r.Delete("/connections/{id}", inConnectionHandler.Revoke)
		})
	})
}
