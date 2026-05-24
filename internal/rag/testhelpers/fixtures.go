package testhelpers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// randSuffix returns a short hex identifier so concurrent tests do not
// collide on the Hivy `unique` constraints (e.g. Org.Name,
// User.Email, Integration.UniqueKey).
func randSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b)
}

// NewTestOrg inserts a real Org row and registers a t.Cleanup that
// deletes that org plus anything tied to it via org_id cascades.
// Mirrors the per-test cleanup pattern of
// internal/middleware/integration_test.go:63-69, but covers the
// superset of tables RAG touches.
func NewTestOrg(t *testing.T, db *gorm.DB) *model.Org {
	t.Helper()

	org := &model.Org{
		Name:   fmt.Sprintf("test-org-%s", randSuffix(t)),
		Active: true,
	}
	if err := db.Create(org).Error; err != nil {
		t.Fatalf("NewTestOrg: create org: %v", err)
	}

	t.Cleanup(func() {
		// Delete direct dependents before the org itself. Hivy uses
		// a mix of ON DELETE CASCADE (modern models) and no FK action
		// (older ones), so we belt-and-suspenders the common
		// RAG-adjacent tables.
		db.Where("org_id = ?", org.ID).Delete(&model.Connection{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Where("org_id = ?", org.ID).Delete(&model.AuditEntry{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	return org
}

// NewTestUser inserts a real User row, plus an OrgMembership with
// Role="owner" tying that user to orgID. Registers t.Cleanup for both.
func NewTestUser(t *testing.T, db *gorm.DB, orgID uuid.UUID) *model.User {
	t.Helper()

	suffix := randSuffix(t)
	now := time.Now()
	user := &model.User{
		Email:            fmt.Sprintf("user-%s@test.hivy", suffix),
		Name:             "Test User " + suffix,
		EmailConfirmedAt: &now,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("NewTestUser: create user: %v", err)
	}

	membership := &model.OrgMembership{
		UserID: user.ID,
		OrgID:  orgID,
		Role:   "owner",
	}
	if err := db.Create(membership).Error; err != nil {
		// Surface create error even though the user is already there.
		t.Fatalf("NewTestUser: create org_membership: %v", err)
	}

	t.Cleanup(func() {
		db.Where("id = ?", membership.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})

	return user
}

// NewTestIntegration inserts a real Integration row. Provider is a
// free-form source key (e.g. "github", "notion"); UniqueKey is
// auto-generated with a random suffix so repeated calls in a single test
// binary don't collide.
func NewTestIntegration(t *testing.T, db *gorm.DB, provider string) *model.Integration {
	t.Helper()

	if provider == "" {
		provider = "test-provider"
	}
	suffix := randSuffix(t)
	integ := &model.Integration{
		UniqueKey:   fmt.Sprintf("%s-%s", provider, suffix),
		Provider:    fmt.Sprintf("%s-%s", provider, suffix),
		DisplayName: fmt.Sprintf("Test %s Integration (%s)", provider, suffix),
	}
	if err := db.Create(integ).Error; err != nil {
		t.Fatalf("NewTestIntegration: %v", err)
	}

	t.Cleanup(func() {
		// Connection rows referencing this integration are expected to
		// be cleaned up by their owning org's cleanup (and by CASCADE).
		// We then drop the integration itself.
		db.Where("id = ?", integ.ID).Delete(&model.Integration{})
	})

	return integ
}

// NewTestConnection inserts a real Connection tying a user to an
// integration inside an org, with a fake Nango connection ID.
func NewTestConnection(t *testing.T, db *gorm.DB, orgID, userID, integrationID uuid.UUID) *model.Connection {
	t.Helper()

	conn := &model.Connection{
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integrationID,
		NangoConnectionID: fmt.Sprintf("nango-fake-%s", randSuffix(t)),
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatalf("NewTestConnection: %v", err)
	}

	t.Cleanup(func() {
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
	})

	return conn
}

// NewTestRAGSource inserts a real RAGSource of kind INTEGRATION tying
// an Connection to the RAG source model. Tests that need to
// satisfy the rag_source_id FK on any child RAG table should call
// this helper to set one up.
//
// Registers a t.Cleanup that removes the RAGSource row — every child
// table has ON DELETE CASCADE on rag_source_id, so this single delete
// sweeps any sync-state / attempt / junction rows the test created.
func NewTestRAGSource(
	t *testing.T,
	db *gorm.DB,
	orgID, connectionID uuid.UUID,
) *ragmodel.RAGSource {
	t.Helper()

	connID := connectionID
	src := &ragmodel.RAGSource{
		OrgIDValue:   orgID,
		KindValue:    ragmodel.RAGSourceKindIntegration,
		Name:         fmt.Sprintf("test-rag-source-%s", randSuffix(t)),
		Status:       ragmodel.RAGSourceStatusActive,
		Enabled:      true,
		ConnectionID: &connID,
		AccessType:   ragmodel.AccessTypePrivate,
	}
	if err := db.Create(src).Error; err != nil {
		t.Fatalf("NewTestRAGSource: %v", err)
	}

	t.Cleanup(func() {
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})

	return src
}
