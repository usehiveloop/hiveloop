package github

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

type Stage string

const (
	StageStart  Stage = "START"
	StagePRs    Stage = "PRS"
	StageIssues Stage = "ISSUES"
	StageDone   Stage = "DONE"
)

func (s Stage) IsValid() bool {
	switch s {
	case StageStart, StagePRs, StageIssues, StageDone:
		return true
	default:
		return false
	}
}

// GithubCheckpoint embeds interfaces.AnyCheckpoint to satisfy the
// Checkpoint marker — the unexported isCheckpoint() method can only be
// implemented by types in the interfaces package, so embedding is the
// cross-package conformance pattern. Completion is signalled via
// Stage = DONE; HasMore is unused.
type GithubCheckpoint struct {
	interfaces.AnyCheckpoint

	Stage               Stage      `json:"stage"`
	RepoIDsRemaining    []string   `json:"repo_full_names_remaining,omitempty"`
	CurrentRepoID       *int64     `json:"current_repo_id,omitempty"`
	CurrentRepoFullName *string    `json:"current_repo_full_name,omitempty"`
	CurrPage            int        `json:"curr_page"`
	LastSeenUpdatedAt   *time.Time `json:"last_seen_updated_at,omitempty"`
}

func dummyCheckpoint() GithubCheckpoint {
	return GithubCheckpoint{Stage: StageStart, CurrPage: 1}
}

// Returns an error on malformed JSON or unknown Stage so the scheduler
// escalates to a fresh re-run rather than silently advancing into an
// unknown state.
func unmarshalCheckpoint(raw json.RawMessage) (GithubCheckpoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return dummyCheckpoint(), nil
	}
	var cp GithubCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return GithubCheckpoint{}, fmt.Errorf("github: parse checkpoint: %w", err)
	}
	if cp.Stage == "" {
		cp.Stage = StageStart
	}
	if !cp.Stage.IsValid() {
		return GithubCheckpoint{}, fmt.Errorf("github: invalid stage %q", cp.Stage)
	}
	if cp.CurrPage < 1 {
		cp.CurrPage = 1
	}
	return cp, nil
}
