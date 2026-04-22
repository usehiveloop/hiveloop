package model_test

import (
	"errors"
	"testing"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// intPtr avoids the Go-wart of not being able to take the address of
// an integer literal inline.
func intPtr(i int) *int { return &i }

// TestRAGConnectionConfig_ValidateRefreshFreq — port of Onyx's branch
// at backend/onyx/db/models.py:1916-1921.
//
// Business value: this is the admin-API input guard. Reject cadences
// that would pulverize source APIs or race the ingest loop itself.
func TestRAGConnectionConfig_ValidateRefreshFreq(t *testing.T) {
	cases := []struct {
		name    string
		freq    *int
		wantErr error
	}{
		{"nil-freq-is-valid", nil, nil},
		{"below-minimum-59s-rejected", intPtr(59), ragmodel.ErrRefreshFreqTooSmall},
		{"minimum-60s-accepted", intPtr(60), nil},
		{"one-hour-accepted", intPtr(3600), nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := &ragmodel.RAGConnectionConfig{RefreshFreqSeconds: tc.freq}
			got := cfg.ValidateRefreshFreq()
			if tc.wantErr == nil {
				if got != nil {
					t.Fatalf("freq=%v: expected nil error, got %v", tc.freq, got)
				}
				return
			}
			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("freq=%v: expected %v, got %v", tc.freq, tc.wantErr, got)
			}
		})
	}
}

// TestRAGConnectionConfig_ValidatePruneFreq — port of Onyx's branch
// at backend/onyx/db/models.py:1923-1928.
func TestRAGConnectionConfig_ValidatePruneFreq(t *testing.T) {
	cases := []struct {
		name    string
		freq    *int
		wantErr error
	}{
		{"nil-freq-is-valid", nil, nil},
		{"below-minimum-299s-rejected", intPtr(299), ragmodel.ErrPruneFreqTooSmall},
		{"minimum-300s-accepted", intPtr(300), nil},
		{"one-day-accepted", intPtr(86400), nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := &ragmodel.RAGConnectionConfig{PruneFreqSeconds: tc.freq}
			got := cfg.ValidatePruneFreq()
			if tc.wantErr == nil {
				if got != nil {
					t.Fatalf("freq=%v: expected nil error, got %v", tc.freq, got)
				}
				return
			}
			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("freq=%v: expected %v, got %v", tc.freq, tc.wantErr, got)
			}
		})
	}
}
