package handler

import (
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func setAuditDiff(r *http.Request, old map[string]any, updates map[string]any) {
	changes := middleware.AdminAuditChanges{}
	for field, newVal := range updates {
		oldVal, exists := old[field]
		if !exists || fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal) {
			changes[field] = map[string]any{"old": oldVal, "new": newVal}
		}
	}
	if len(changes) > 0 {
		middleware.SetAdminAuditChanges(r, changes)
	}
}

type AdminHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	nango        *nango.Client
	catalog      *catalog.Catalog
	enqueuer     enqueue.TaskEnqueuer

	privateKey *rsa.PrivateKey
	signingKey []byte
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewAdminHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	nangoClient *nango.Client,
	cat *catalog.Catalog,
	privateKey *rsa.PrivateKey,
	signingKey []byte,
	issuer, audience string,
	accessTTL, refreshTTL time.Duration,
	enqueuer enqueue.TaskEnqueuer,
) *AdminHandler {
	return &AdminHandler{
		db:           db,
		orchestrator: orchestrator,
		nango:        nangoClient,
		catalog:      cat,
		enqueuer:     enqueuer,
		privateKey:   privateKey,
		signingKey:   signingKey,
		issuer:       issuer,
		audience:     audience,
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
	}
}

// Stats handles GET /admin/v1/stats.
// @Summary Platform stats
// @Description Returns platform-wide aggregate statistics.
// @Tags admin
// @Produce json
// @Success 200 {object} adminStatsResponse
// @Security BearerAuth
// @Router /admin/v1/stats [get]
func (h *AdminHandler) Stats(w http.ResponseWriter, r *http.Request) {
	var stats adminStatsResponse

	h.db.Model(&model.User{}).Count(&stats.TotalUsers)
	h.db.Model(&model.Org{}).Count(&stats.TotalOrgs)
	h.db.Model(&model.Agent{}).Where("status = ?", "active").Count(&stats.TotalAgents)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "running").Count(&stats.TotalSandboxesRunning)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "stopped").Count(&stats.TotalSandboxesStopped)
	h.db.Model(&model.Sandbox{}).Where("status = ?", "error").Count(&stats.TotalSandboxesError)
	h.db.Model(&model.Generation{}).Count(&stats.TotalGenerations)
	h.db.Model(&model.AgentConversation{}).Where("status = ?", "active").Count(&stats.TotalConversationsActive)
	h.db.Model(&model.Credential{}).Where("revoked_at IS NULL").Count(&stats.TotalCredentials)

	var costResult struct{ Total float64 }
	h.db.Model(&model.Generation{}).Select("COALESCE(SUM(cost), 0) as total").Scan(&costResult)
	stats.TotalCost = costResult.Total

	writeJSON(w, http.StatusOK, stats)
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
