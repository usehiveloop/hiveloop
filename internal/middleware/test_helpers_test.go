package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/model"
)

const (
	testDBURL      = "postgres://hivy:localdev@localhost:5433/hivy_test?sslmode=disable" // #nosec G101 -- local test DB fixture
	testSigningKey = "local-dev-signing-key-change-in-prod"
)

// connectTestDB opens a real Postgres connection and runs migrations.
func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}

	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// cleanupOrg deletes a test org and its dependents after the test.
func cleanupOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.AuditEntry{})
	db.Where("org_id = ?", orgID).Delete(&model.Token{})
	db.Where("org_id = ?", orgID).Delete(&model.Credential{})
	db.Where("id = ?", orgID).Delete(&model.Org{})
}

// authTestHelper manages RSA key pairs and JWT minting for tests.
type authTestHelper struct {
	privKey  *rsa.PrivateKey
	issuer   string
	audience string
}

func newAuthHelper(t *testing.T) *authTestHelper {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return &authTestHelper{
		privKey:  privKey,
		issuer:   "test-issuer",
		audience: "test-audience",
	}
}

// createTestOrg creates an Hivy Org in Postgres and mints a JWT for it.
func (ah *authTestHelper) createTestOrg(t *testing.T, db *gorm.DB, name, role string) (model.Org, string) {
	t.Helper()

	uniqueName := fmt.Sprintf("%s-%s", name, uuid.New().String()[:8])
	orgID := uuid.New()
	userID := uuid.New().String()

	org := model.Org{
		ID:        orgID,
		Name:      uniqueName,
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org in DB: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	jwtToken, err := auth.IssueAccessToken(ah.privKey, ah.issuer, ah.audience, userID, orgID.String(), role, time.Hour)
	if err != nil {
		t.Fatalf("failed to issue access token: %v", err)
	}

	return org, jwtToken
}
