package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecode_Cursor_Roundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Nanosecond)
	id := uuid.New()

	encoded := encodeCursor(now, id)
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !decoded.CreatedAt.Equal(now) {
		t.Fatalf("expected CreatedAt=%v, got %v", now, decoded.CreatedAt)
	}
	if decoded.ID != id {
		t.Fatalf("expected ID=%s, got %s", id, decoded.ID)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not base64", "!!!invalid!!!"},
		{"no separator", "bm9zZXBhcmF0b3I="},                       // "noseparator"
		{"bad time", "YmFkLXRpbWV8MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAw"}, // "bad-time|00000000-..."
		{"bad uuid", "MjAyNi0wMS0wMVQwMDowMDowMFp8bm90LWEtdXVpZA=="},                  // "2026-01-01T00:00:00Z|not-a-uuid"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeCursor(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	limit, cursor, err := parsePagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 50 {
		t.Fatalf("expected default limit=50, got %d", limit)
	}
	if cursor != nil {
		t.Fatal("expected nil cursor by default")
	}
}

func TestParsePagination_CustomLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=10", nil)
	limit, _, err := parsePagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 10 {
		t.Fatalf("expected limit=10, got %d", limit)
	}
}

func TestParsePagination_LimitCapped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=500", nil)
	limit, _, err := parsePagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 100 {
		t.Fatalf("expected limit capped to 100, got %d", limit)
	}
}

func TestParsePagination_InvalidLimit(t *testing.T) {
	tests := []string{"0", "-1", "abc"}
	for _, l := range tests {
		req := httptest.NewRequest(http.MethodGet, "/v1/credentials?limit="+l, nil)
		_, _, err := parsePagination(req)
		if err == nil {
			t.Fatalf("expected error for limit=%s, got nil", l)
		}
	}
}

func TestParsePagination_WithCursor(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()
	c := encodeCursor(now, id)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?cursor="+c, nil)
	_, cursor, err := parsePagination(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor == nil {
		t.Fatal("expected cursor to be parsed")
	}
	if cursor.ID != id {
		t.Fatalf("expected cursor ID=%s, got %s", id, cursor.ID)
	}
}

func TestParsePagination_InvalidCursor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?cursor=garbage", nil)
	_, _, err := parsePagination(req)
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
}
