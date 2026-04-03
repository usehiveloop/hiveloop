package forge

import (
	"testing"
)

func TestMockMatching_ExactMatch(t *testing.T) {
	samples := []MockSample{
		{Match: map[string]any{"order_id": "123"}, Response: map[string]any{"status": "delivered"}},
		{Match: map[string]any{}, Response: map[string]any{"error": "not found"}},
	}

	t.Run("exact_match_returns_first_sample", func(t *testing.T) {
		args := map[string]any{"order_id": "123"}
		result := findBestMock(args, samples)
		status, ok := result.Response["status"]
		if !ok || status != "delivered" {
			t.Errorf("expected {status: delivered}, got %v", result.Response)
		}
	})

	t.Run("no_match_returns_wildcard", func(t *testing.T) {
		args := map[string]any{"order_id": "456"}
		result := findBestMock(args, samples)
		errVal, ok := result.Response["error"]
		if !ok || errVal != "not found" {
			t.Errorf("expected {error: not found}, got %v", result.Response)
		}
	})

	t.Run("empty_args_returns_wildcard", func(t *testing.T) {
		args := map[string]any{}
		result := findBestMock(args, samples)
		errVal, ok := result.Response["error"]
		if !ok || errVal != "not found" {
			t.Errorf("expected {error: not found}, got %v", result.Response)
		}
	})
}

func TestMockMatching_PartialMatch(t *testing.T) {
	samples := []MockSample{
		{Match: map[string]any{"order_id": "123", "customer_id": "456"}, Response: map[string]any{"full_match": true}},
		{Match: map[string]any{"order_id": "123"}, Response: map[string]any{"partial_match": true}},
	}

	t.Run("both_keys_match_returns_full_match", func(t *testing.T) {
		args := map[string]any{"order_id": "123", "customer_id": "456"}
		result := findBestMock(args, samples)
		if v, ok := result.Response["full_match"]; !ok || v != true {
			t.Errorf("expected {full_match: true}, got %v", result.Response)
		}
	})

	t.Run("one_key_match_returns_partial_match", func(t *testing.T) {
		args := map[string]any{"order_id": "123"}
		result := findBestMock(args, samples)
		if v, ok := result.Response["partial_match"]; !ok || v != true {
			t.Errorf("expected {partial_match: true}, got %v", result.Response)
		}
	})
}

func TestMockMatching_NoMatch_FallsBackToFirst(t *testing.T) {
	samples := []MockSample{
		{Match: map[string]any{"x": "1"}, Response: map[string]any{"first": true}},
		{Match: map[string]any{"y": "2"}, Response: map[string]any{"second": true}},
	}

	args := map[string]any{"z": "3"}
	result := findBestMock(args, samples)
	if v, ok := result.Response["first"]; !ok || v != true {
		t.Errorf("expected fallback to first sample {first: true}, got %v", result.Response)
	}
}

func TestMockMatching_EmptySamples(t *testing.T) {
	result := findBestMock(map[string]any{"a": "1"}, nil)
	if result.Match != nil || result.Response != nil {
		t.Errorf("expected zero-value MockSample, got match=%v response=%v", result.Match, result.Response)
	}

	result2 := findBestMock(map[string]any{}, []MockSample{})
	if result2.Match != nil || result2.Response != nil {
		t.Errorf("expected zero-value MockSample for empty slice, got match=%v response=%v", result2.Match, result2.Response)
	}
}
