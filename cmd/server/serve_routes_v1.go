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
	subscriptionHandler *handler.SubscriptionHandler,
	credHandler *handler.CredentialHandler,
	tokenHandler *handler.TokenHandler,
	sandboxTemplateHandler *handler.SandboxTemplateHandler,
	skillHandler *handler.SkillHandler,
	agentHandler *handler.AgentHandler,
	conversationHandler *handler.ConversationHandler,
	customDomainHandler *handler.CustomDomainHandler,
	ragSourceHandler *handler.RAGSourceHandler,
	ragSearchHandler *handler.RAGSearchHandler,
	uploadsHandler *handler.UploadsHandler,
	systemTaskHandler *handler.SystemTaskHandler,
	employeeHandler *handler.EmployeeHandler,
	chatHandler *handler.ChatHandler,
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
				r.Patch("/orgs/current", orgHandler.Update)
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

			mountBillingRoutes(r, billingHandler, subscriptionHandler)

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
				})
				triggerDeliveryHandler := handler.NewTriggerDeliveryHandler(database)
				_ = agentHandler
				if employeeHandler != nil {
					r.Get("/employees", employeeHandler.List)
					r.Get("/employees/{id}", employeeHandler.Get)
					r.Get("/employees/{id}/sessions", employeeHandler.ListSessions)
					if conversationHandler != nil {
						r.Post("/employees/{id}/sessions", conversationHandler.CreateEmployeeSession)
						r.Get("/employees/{id}/sessions/{convID}", conversationHandler.Get)
					}
					r.Get("/employees/{id}/specialists", employeeHandler.ListSpecialists)
					r.Post("/employees/{id}/specialists/{slug}", employeeHandler.EnableSpecialist)
					r.Delete("/employees/{id}/specialists/{slug}", employeeHandler.DisableSpecialist)
					r.Route("/employees/{id}/skills", func(r chi.Router) {
						r.Post("/", skillHandler.AttachToEmployee)
						r.Get("/", skillHandler.ListEmployeeSkills)
						r.Delete("/{skillID}", skillHandler.DetachFromEmployee)
					})
					r.Get("/employees/{id}/trigger-deliveries", triggerDeliveryHandler.List)
					r.Get("/employees/{id}/trigger-deliveries/{deliveryID}", triggerDeliveryHandler.Get)
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireOrgAdmin(database))
						r.Post("/employees/{id}/sync", employeeHandler.Sync)
						r.Post("/employees/{id}/sandbox/upgrade", employeeHandler.StartSandboxUpgrade)
						r.Get("/employees/{id}/sandbox/upgrades/{upgradeID}", employeeHandler.GetSandboxUpgrade)
					})
				}
				if chatHandler != nil {
					r.Group(func(r chi.Router) {
						r.Use(middleware.ResolveUser(database))
						r.Post("/employees/{id}/chats", chatHandler.Create)
						r.Get("/chats", chatHandler.List)
						r.Get("/chats/{id}", chatHandler.Get)
						r.Post("/chats/{id}/messages", chatHandler.Send)
					})
				}
				if systemTaskHandler != nil {
					r.Post("/system/tasks/{taskName}", systemTaskHandler.Run)
				}
				if conversationHandler != nil {
					r.Route("/conversations/{convID}", func(r chi.Router) {
						r.Get("/", conversationHandler.Get)
						r.Delete("/", conversationHandler.End)
						r.Get("/messages", conversationHandler.ListMessages)
						r.Post("/messages", conversationHandler.SendMessage)
						r.Get("/stream", conversationHandler.Stream)
						r.Get("/history", conversationHandler.History)
						r.Post("/abort", conversationHandler.Abort)
						r.Get("/approvals", conversationHandler.ListApprovals)
						r.Post("/approvals/{requestID}", conversationHandler.ResolveApproval)
						r.Get("/events", conversationHandler.ListEvents)
						r.Get("/assets", conversationHandler.ListAssets)
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
					// TODO: tighten back to RequireOrgAdmin once the RAG
					// admin UI is admin-gated.
					r.Use(middleware.ResolveUser(database))
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
					if ragSearchHandler != nil {
						r.Post("/search", ragSearchHandler.Search)
					}
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
				r.Get("/assets", uploadsHandler.ListAssets)
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
