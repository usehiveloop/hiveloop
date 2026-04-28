// Package main runs an in-process Nango fake for sandboxed agent test runs.
// No verification of bearer tokens, HMACs, or session TTLs — every inbound
// request is trusted. Outbound webhook signatures ARE generated because the
// real backend at internal/handler/nango_webhooks.go verifies them.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	addr := flag.String("addr", ":3004", "listen address")
	wsPath := flag.String("ws-path", "/", "WebSocket upgrade path")
	secret := flag.String("secret", envOr("NANGO_WEBHOOK_SECRET", "fake-nango-secret"), "secret used to sign outbound webhooks")
	target := flag.String("webhook-target", envOr("WEBHOOK_TARGET", "http://localhost:8080/internal/webhooks/nango"), "default URL for forward/auth webhooks")
	flag.Parse()

	st := newStore()
	hub := newHub()
	wh := newWebhookSender(*secret, *target)

	r := chi.NewRouter()
	r.Use(logRequest)

	r.HandleFunc(*wsPath, hub.handle)

	mountProviders(r, st)
	mountIntegrations(r, st)
	mountConnections(r, st)
	mountConnectSessions(r, st)
	mountProxy(r, st)
	mountOAuth(r, st, hub, wh)
	mountFormAuth(r, st, wh)
	mountAdmin(r, st, hub, wh)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("fake-nango listening", "addr", *addr, "ws_path", *wsPath, "webhook_target", *target)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("req", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
