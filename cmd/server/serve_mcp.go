package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bootstrap"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/mcpserver"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
)

func setupMCPServer(
	ctx context.Context,
	cfg *config.Config,
	deps *bootstrap.Deps,
	signingKey []byte,
	database *gorm.DB,
	mcpHandler *handler.MCPHandler,
) *http.Server {
	mcpRouter := chi.NewRouter()
	mcpRouter.Use(chimw.RequestID)
	mcpRouter.Use(chimw.RealIP)
	mcpRouter.Use(posthogobs.Recoverer(deps.PostHog))
	mcpRouter.Use(middleware.RequestLog(slog.Default()))

	replyMCPHandler := mcpserver.NewReplyMCPHandler(database, deps.ActionsCatalog)
	mcpRouter.Route("/reply/{connectionID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", replyMCPHandler.StreamableHTTPHandler())
		r.Handle("/", replyMCPHandler.StreamableHTTPHandler())
	})
	slog.Info("hiveloop reply MCP registered on /reply/{connectionID}")

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
		ErrorLog:     posthogobs.NewStdlogBridge("mcp_server"),
	}

	mcpHandler.ServerCache.StartCleanup(ctx, 5*time.Minute)

	goroutine.Go(func() {
		slog.Info("mcp server starting", "port", cfg.MCPPort)
		if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server error", "error", err)
		}
	})

	return mcpSrv
}
