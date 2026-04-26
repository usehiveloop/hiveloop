package github

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCheckpoint_MarshalRoundTrip(t *testing.T) {
	repoID := int64(42)
	repoName := "acme/widget"
	seen := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	original := GithubCheckpoint{
		Stage:               StagePRs,
		RepoIDsRemaining:    []string{"acme/gadget", "acme/sprocket"},
		CurrentRepoID:       &repoID,
		CurrentRepoFullName: &repoName,
		CurrPage:            3,
		LastSeenUpdatedAt:   &seen,
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	got, err := unmarshalCheckpoint(raw)
	if err != nil {
		t.Fatalf("unmarshalCheckpoint: %v", err)
	}
	// Time round-trip is byte-exact when both ends use UTC; the JSON
	// tag on the field uses RFC3339Nano so this holds.
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round-trip mismatch:\n got = %+v\n want = %+v", got, original)
	}
}

func TestCheckpoint_DummyZeroValueIsValid(t *testing.T) {
	cp := dummyCheckpoint()
	if cp.Stage != StageStart {
		t.Fatalf("dummy Stage = %q, want %q", cp.Stage, StageStart)
	}
	if cp.CurrPage != 1 {
		t.Fatalf("dummy CurrPage = %d, want 1", cp.CurrPage)
	}
}

func TestCheckpoint_UnmarshalEmptyAndNullProduceDummy(t *testing.T) {
	for _, in := range []string{``, `null`} {
		cp, err := unmarshalCheckpoint(json.RawMessage(in))
		if err != nil {
			t.Fatalf("unmarshalCheckpoint(%q): %v", in, err)
		}
		if cp.Stage != StageStart || cp.CurrPage != 1 {
			t.Fatalf("expected dummy on input %q, got %+v", in, cp)
		}
	}
}

func TestCheckpoint_UnmarshalRejectsUnknownStage(t *testing.T) {
	_, err := unmarshalCheckpoint(json.RawMessage(`{"stage":"COMMENTS"}`))
	if err == nil {
		t.Fatal("expected error on unknown stage; got nil")
	}
}
