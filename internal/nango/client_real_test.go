package nango_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestRealNangoCreateIntegrationRequiresConcreteOAuthCredentials(t *testing.T) {
	endpoint := "http://localhost:23003"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	secret := discoverRealNangoSecret(t, ctx)

	client := nango.NewClient(endpoint, secret)
	uniqueKey := fmt.Sprintf("in_linear-profile-realtest-%d", time.Now().UnixNano())
	err := client.CreateIntegration(ctx, nango.CreateIntegrationRequest{
		UniqueKey:   uniqueKey,
		Provider:    "linear",
		DisplayName: "Real Nango Placeholder Test",
		Credentials: &nango.Credentials{
			Type:         "OAUTH2",
			ClientID:     "hivy-placeholder-client-id-8f47c2d91b6a",
			ClientSecret: "hivy-placeholder-client-secret-3a91e58c0d74",
			Scopes:       "comments:create,comments:read",
		},
	})
	if err != nil {
		t.Fatalf("create integration with placeholder OAuth credentials: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.DeleteIntegration(ctx, uniqueKey); err != nil {
			t.Logf("cleanup delete integration %s: %v", uniqueKey, err)
		}
	})

	missingKey := fmt.Sprintf("in_linear-profile-realtest-missing-%d", time.Now().UnixNano())
	err = client.CreateIntegration(ctx, nango.CreateIntegrationRequest{
		UniqueKey:   missingKey,
		Provider:    "linear",
		DisplayName: "Real Nango Missing Credentials Test",
		Credentials: &nango.Credentials{
			Type:   "OAUTH2",
			Scopes: "comments:create,comments:read",
		},
	})
	if err == nil {
		_ = client.DeleteIntegration(ctx, missingKey)
		t.Fatal("expected Nango to reject OAuth credentials without client_id/client_secret")
	}
	if !strings.Contains(err.Error(), "client_id") || !strings.Contains(err.Error(), "client_secret") {
		t.Fatalf("expected client_id/client_secret validation error, got %v", err)
	}
}

func discoverRealNangoSecret(t *testing.T, ctx context.Context) string {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.NangoDatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Nango database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get Nango database handle: %v", err)
	}
	defer sqlDB.Close()

	var secret string
	if err := db.WithContext(ctx).
		Raw(`SELECT secret_key FROM nango._nango_environments WHERE name = ? LIMIT 1`, "prod").
		Scan(&secret).Error; err != nil {
		t.Fatalf("query Nango secret: %v", err)
	}
	if strings.TrimSpace(secret) == "" {
		t.Fatal("Nango prod secret is empty")
	}
	return secret
}
