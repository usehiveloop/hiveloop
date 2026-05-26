package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOAuth_Login_SetsLocalNextCookie(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	next := "/invites/accept?token=test-token&auto=1"
	req := httptest.NewRequest(http.MethodGet, "/oauth/github?next="+url.QueryEscape(next), nil)
	rr := httptest.NewRecorder()

	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	var nextCookie *http.Cookie
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == "oauth_next" {
			nextCookie = cookie
			break
		}
	}
	if nextCookie == nil {
		t.Fatal("expected oauth_next cookie")
	}

	got, err := url.QueryUnescape(nextCookie.Value)
	if err != nil {
		t.Fatalf("decode oauth_next cookie: %v", err)
	}
	if got != next {
		t.Fatalf("expected next %q, got %q", next, got)
	}
}

func TestOAuth_Login_IgnoresExternalNext(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/github?next="+url.QueryEscape("https://evil.test"), nil)
	rr := httptest.NewRecorder()

	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rr.Code)
	}

	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == "oauth_next" {
			t.Fatal("did not expect oauth_next cookie for external redirect")
		}
	}
}
