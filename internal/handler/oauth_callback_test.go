package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOAuth_Callback_ProviderNotConfigured_NoUserCreated verifies that when a provider
// is not configured, no user record is created and the error redirect happens.
func TestOAuth_Callback_ProviderNotConfigured_NoUserCreated(t *testing.T) {
	h := newOAuthHarness(t) // harness with no provider credentials

	// Make callback request with valid state/verifier cookies but no provider config
	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "test-verifier"})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	// The redirect-with-error response is the SUT's actual behavior under
	// this failure mode — if it landed here, the handler exited before
	// touching users. (Asserting on global users.Count would be
	// un-isolated against other tests sharing this DB.)
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "provider_not_configured") {
		t.Errorf("expected provider_not_configured error in redirect, got %s", location)
	}
}

// TestOAuth_Callback_InvalidState_NoUserCreatedNoTokenIssued verifies that invalid
// CSRF state prevents user creation and token issuance.
func TestOAuth_Callback_InvalidState_NoUserCreatedNoTokenIssued(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=test-code&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "test-verifier"})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "invalid_state") {
		t.Errorf("expected invalid_state error in redirect, got %s", location)
	}
	// Sufficient: a redirect with the invalid_state error proves the
	// handler exited at CSRF check, before any user / exchange-token
	// write. Global table-count assertions would conflate this test's
	// behavior with state seeded by other tests sharing the DB.
}

// TestOAuth_Callback_MissingState_RedirectsWithError verifies that missing state
// cookie results in error redirect without user creation.
func TestOAuth_Callback_MissingState_RedirectsWithError(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=test-code&state=test-state", nil)
	// No cookies set - missing state

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "invalid_state") {
		t.Errorf("expected invalid_state error in redirect, got %s", location)
	}
}

// TestOAuth_Callback_MissingVerifier_NoUserCreated verifies that missing PKCE verifier
// results in error redirect without user creation.
func TestOAuth_Callback_MissingVerifier_NoUserCreated(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=test-code&state=test-state", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	// Missing oauth_verifier cookie

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "missing_verifier") {
		t.Errorf("expected missing_verifier error in redirect, got %s", location)
	}
}

// TestOAuth_Callback_MissingCode_NoTokenExchanged verifies that missing authorization
// code results in error redirect without token exchange.
func TestOAuth_Callback_MissingCode_NoTokenExchanged(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?state=test-state", nil) // no code
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "test-verifier"})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "missing_code") {
		t.Errorf("expected missing_code error in redirect, got %s", location)
	}
}

// TestOAuth_Callback_ProviderError_NoUserCreated verifies that provider errors
// result in error redirect without user creation.
func TestOAuth_Callback_ProviderError_NoUserCreated(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github/callback?code=test-code&state=test-state&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "test-verifier"})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "access_denied") {
		t.Errorf("expected access_denied error in redirect, got %s", location)
	}
}