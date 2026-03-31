package turso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTursoServer creates an httptest server that mimics the Turso Platform API.
func mockTursoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		// Create database
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/databases"):
			var body struct {
				Name  string `json:"name"`
				Group string `json:"group"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Name == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"name required"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{
					"Name":     body.Name,
					"DbId":     "db-" + body.Name,
					"Hostname": body.Name + "-test-org.turso.io",
				},
			})

		// Create token
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/auth/tokens"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"jwt": "eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.mock-token",
			})

		// Get database
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/databases/"):
			parts := strings.Split(r.URL.Path, "/")
			dbName := parts[len(parts)-1]
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"database": map[string]any{
					"Name":     dbName,
					"DbId":     "db-" + dbName,
					"Hostname": dbName + "-test-org.turso.io",
				},
			})

		// Delete database
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/databases/"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"database": "deleted"})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newTestClient creates a Client pointed at the mock server.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient("test-token", "test-org")
	c.baseURL = srv.URL
	return c
}

func TestCreateDatabase(t *testing.T) {
	srv := mockTursoServer(t)
	defer srv.Close()
	c := newTestClient(srv)

	db, err := c.CreateDatabase(context.Background(), "my-db", "default")
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if db.Name != "my-db" {
		t.Errorf("name: got %q", db.Name)
	}
	if db.Hostname != "my-db-test-org.turso.io" {
		t.Errorf("hostname: got %q", db.Hostname)
	}
	if db.DbID != "db-my-db" {
		t.Errorf("dbid: got %q", db.DbID)
	}
}

func TestGetDatabase(t *testing.T) {
	srv := mockTursoServer(t)
	defer srv.Close()
	c := newTestClient(srv)

	db, err := c.GetDatabase(context.Background(), "my-db")
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	if db.Name != "my-db" {
		t.Errorf("name: got %q", db.Name)
	}
}

func TestCreateToken(t *testing.T) {
	srv := mockTursoServer(t)
	defer srv.Close()
	c := newTestClient(srv)

	token, err := c.CreateToken(context.Background(), "my-db")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Errorf("expected JWT-like token, got %q", token)
	}
}

func TestDeleteDatabase(t *testing.T) {
	srv := mockTursoServer(t)
	defer srv.Close()
	c := newTestClient(srv)

	err := c.DeleteDatabase(context.Background(), "my-db")
	if err != nil {
		t.Fatalf("DeleteDatabase: %v", err)
	}
}

func TestCreateDatabase_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"database already exists"}`))
	}))
	defer srv.Close()

	c := NewClient("token", "org")
	c.baseURL = srv.URL
	_, err := c.CreateDatabase(context.Background(), "existing-db", "default")
	if err == nil {
		t.Fatal("expected error for conflict")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestDatabaseURL(t *testing.T) {
	url := DatabaseURL("my-db-test-org.turso.io")
	if url != "libsql://my-db-test-org.turso.io" {
		t.Errorf("url: got %q", url)
	}
}

func TestAuthHeaderSent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"database": map[string]any{"Name": "x", "DbId": "x", "Hostname": "x.turso.io"},
		})
	}))
	defer srv.Close()

	c := NewClient("secret-key", "org")
	c.baseURL = srv.URL
	_, err := c.GetDatabase(context.Background(), "x")
	if err != nil {
		t.Fatalf("expected auth to work: %v", err)
	}
}
