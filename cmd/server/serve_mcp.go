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

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcpserver"
	"github.com/usehivy/hivy/internal/middleware"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
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
	mcpRouter.Use(sentryobs.Middleware())
	mcpRouter.Use(sentryobs.Recoverer())
	mcpRouter.Use(middleware.RequestLog(slog.Default()))

	replyMCPHandler := mcpserver.NewReplyMCPHandler(database, deps.ActionsCatalog)
	mcpRouter.Route("/reply/{connectionID}", func(r chi.Router) {
		r.Use(middleware.TokenAuth(signingKey, database))
		r.Handle("/*", replyMCPHandler.StreamableHTTPHandler())
		r.Handle("/", replyMCPHandler.StreamableHTTPHandler())
	})
	slog.Info("hivy reply MCP registered on /reply/{connectionID}")

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
		ErrorLog:     sentryobs.NewStdlogBridge("mcp_server"),
	}

	mcpHandler.ServerCache.StartCleanup(ctx, 5*time.Minute)

	goroutine.Go(ctx, func(context.Context) {
		slog.Info("mcp server starting", "port", cfg.MCPPort)
		if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("mcp server error", "error", err)
		}
	})

	return mcpSrv
}
