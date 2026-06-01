package employeeruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClientWithTimeoutAppliesHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewClientWithTimeout(srv.URL, "runtime-secret", time.Millisecond)

	err := client.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected healthz to fail when response headers exceed configured timeout")
	}
}
