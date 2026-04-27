package handler

import (
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/counter"
	"github.com/usehiveloop/hiveloop/internal/mcp"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TokenHandler manages sandbox proxy token operations.
type TokenHandler struct {
	db           *gorm.DB
	signingKey   []byte
	cacheManager *cache.Manager
	counter      *counter.Counter
	catalog      *catalog.Catalog
	mcpBaseURL   string
	serverCache  MCPServerCache
}

// MCPServerCache is an interface for evicting cached MCP servers.
type MCPServerCache interface {
	Evict(jti string)
}

// NewTokenHandler creates a new token handler.
func NewTokenHandler(db *gorm.DB, signingKey []byte, cm *cache.Manager, ctr *counter.Counter, cat *catalog.Catalog, mcpBaseURL string, sc MCPServerCache) *TokenHandler {
	return &TokenHandler{db: db, signingKey: signingKey, cacheManager: cm, counter: ctr, catalog: cat, mcpBaseURL: mcpBaseURL, serverCache: sc}
}

type mintTokenRequest struct {
	CredentialID   string           `json:"credential_id"`
	TTL            string           `json:"ttl"` // e.g. "1h", "24h"
	Remaining      *int64           `json:"remaining,omitempty"`
	RefillAmount   *int64           `json:"refill_amount,omitempty"`
	RefillInterval *string          `json:"refill_interval,omitempty"`
	Scopes         []mcp.TokenScope `json:"scopes,omitempty"`
	Meta           model.JSON       `json:"meta,omitempty"`
}

type mintTokenResponse struct {
	Token       string  `json:"token"`
	ExpiresAt   string  `json:"expires_at"`
	JTI         string  `json:"jti"`
	MCPEndpoint *string `json:"mcp_endpoint,omitempty"`
}

const maxTokenTTL = 24 * time.Hour
