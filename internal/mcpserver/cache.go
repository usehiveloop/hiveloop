package mcpserver

import (
	"context"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerCache caches built MCP server instances by JTI to avoid rebuilding on every request.
type ServerCache struct {
	mu      sync.RWMutex
	servers map[string]*cachedServer
}

type cachedServer struct {
	server    *mcp.Server
	expiresAt time.Time
}

// NewServerCache creates a new server cache.
func NewServerCache() *ServerCache {
	return &ServerCache{
		servers: make(map[string]*cachedServer),
	}
}

// GetOrBuild returns a cached server or calls the build function to create one.
// The build function returns the server and its expiry time.
func (c *ServerCache) GetOrBuild(jti string, build func() (*mcp.Server, time.Time, error)) (*mcp.Server, error) {
	c.mu.RLock()
	if cs, ok := c.servers[jti]; ok && time.Now().Before(cs.expiresAt) {
		c.mu.RUnlock()
		return cs.server, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if cs, ok := c.servers[jti]; ok && time.Now().Before(cs.expiresAt) {
		return cs.server, nil
	}

	srv, expiresAt, err := build()
	if err != nil {
		return nil, err
	}

	c.servers[jti] = &cachedServer{
		server:    srv,
		expiresAt: expiresAt,
	}

	return srv, nil
}

// Evict removes a cached server by JTI.
func (c *ServerCache) Evict(jti string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.servers, jti)
}

// StartCleanup runs a background goroutine that removes expired entries periodically.
func (c *ServerCache) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.cleanup()
			}
		}
	}()
}

func (c *ServerCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for jti, cs := range c.servers {
		if now.After(cs.expiresAt) {
			delete(c.servers, jti)
		}
	}
}
