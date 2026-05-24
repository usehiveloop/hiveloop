package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestRailwayProxy_ForwardsRequestAndToken(t *testing.T) {
	var capturedAuth string
	var capturedBody string
	var mu sync.Mutex

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "railway",
			"credentials": map[string]any{
				"access_token": "railway_test_token_abc",
			},
		})
	})

	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedAuth = r.Header.Get("Authorization")
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"me": map[string]any{"name": "Test User"},
			},
		})
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	graphqlBody := `{"query": "query { me { name } }"}`
	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(graphqlBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedAuth != "Bearer railway_test_token_abc" {
		t.Fatalf("expected railway to receive Bearer token, got %q", capturedAuth)
	}

	if capturedBody != graphqlBody {
		t.Fatalf("expected request body forwarded unchanged, got %q", capturedBody)
	}

	var resp map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in response")
	}
	me, ok := data["me"].(map[string]any)
	if !ok {
		t.Fatal("expected me in data")
	}
	if me["name"] != "Test User" {
		t.Fatalf("expected name=Test User, got %v", me["name"])
	}
}

func TestRailwayProxy_CachesTokenByOrg(t *testing.T) {
	nangoCallCount := 0
	var mu sync.Mutex

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		nangoCallCount++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider":    "railway",
			"credentials": map[string]any{"access_token": "cached_railway_token"},
		})
	})

	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	for range 5 {
		req := httptest.NewRequest(http.MethodPost,
			"/internal/railway-proxy/"+harness.agentID.String(),
			bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
		recorder := httptest.NewRecorder()
		harness.router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if nangoCallCount != 1 {
		t.Fatalf("expected nango called once (cached by org), got %d", nangoCallCount)
	}
}

func TestRailwayProxy_InvalidAuth(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called with invalid auth")
	})
	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("railway should not be called with invalid auth")
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
	req.Header.Set("Authorization", "Bearer wrong-key")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRailwayProxy_NoRailwayConnection(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called when no connection exists")
	})
	railwayHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("railway should not be called when no connection exists")
	})

	harness := newRailwayHarness(t, nangoHandler, railwayHandler)

	harness.db.Where("org_id = ?", harness.orgID).Delete(&model.Connection{})

	req := httptest.NewRequest(http.MethodPost,
		"/internal/railway-proxy/"+harness.agentID.String(),
		bytes.NewReader([]byte(`{"query":"query{me{name}}"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
