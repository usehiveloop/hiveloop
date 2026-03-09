package handler

import (
	"log/slog"
	"net/http"
	"net/http/httputil"

	"github.com/useportal/llmvault/internal/cache"
	"github.com/useportal/llmvault/internal/proxy"
)

// NewProxyHandler creates the streaming reverse proxy handler.
// It uses FlushInterval: -1 to immediately flush SSE chunks.
func NewProxyHandler(cacheManager *cache.Manager, transport http.RoundTripper) http.Handler {
	director := proxy.NewDirector(cacheManager)

	rp := &httputil.ReverseProxy{
		Director:      director,
		Transport:     transport,
		FlushInterval: -1, // immediate SSE streaming
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// Check if the director set an error
			if proxyErr := r.Header.Get("X-Proxy-Error"); proxyErr != "" {
				http.Error(w, `{"error":"`+proxyErr+`"}`, http.StatusBadGateway)
				return
			}
			slog.Error("proxy upstream error",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"host", r.URL.Host,
			)
			http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
		},
	}

	return rp
}
