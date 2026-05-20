package nango_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/nango"
)

func TestRealNangoCreateIntegrationRequiresConcreteOAuthCredentials(t *testing.T) {
	if os.Getenv("RUN_REAL_NANGO_TESTS") != "1" {
		t.Skip("set RUN_REAL_NANGO_TESTS=1 to run against docker-compose Nango")
	}
	endpoint := os.Getenv("NANGO_ENDPOINT")
	secret := os.Getenv("NANGO_SECRET_KEY")
	if endpoint == "" || secret == "" {
		t.Fatal("NANGO_ENDPOINT and NANGO_SECRET_KEY are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
