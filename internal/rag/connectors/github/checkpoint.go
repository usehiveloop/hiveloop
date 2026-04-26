// Resumable checkpoint for the GitHub connector.
//
// Stage drives the orchestrator at connector.go: START → PRS → ISSUES →
// DONE. A run that crashes mid-page resumes from CurrentRepoID/CurrPage
// without re-fetching prior pages. RepoIDsRemaining is the queue of
// pending full-names; the orchestrator pops from it as it advances.
//
// Onyx analog: GithubConnectorCheckpoint at backend/onyx/connectors/github/
// models.py — same Stage enum, same paginate-by-repo pattern. We deviate
// only by storing repo identity as (ID, FullName) instead of Onyx's
// SerializedRepository (PyGithub lazy-load workaround), since our types
// are plain structs with no lazy materialisation.
package github

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// Stage is the high-level phase the connector is in within one ingest
// run. The orchestrator transitions linearly; tests assert ordering by
// pinning the expected final stage = DONE on a complete run.
type Stage string

const (
	StageStart   Stage = "START"
	StagePRs     Stage = "PRS"
	StageIssues  Stage = "ISSUES"
	StageDone    Stage = "DONE"
)

// IsValid returns true when s is one of the recognised stages. Used by
// UnmarshalCheckpoint to reject jsonb blobs corrupted by an admin who
// hand-edited the row in production. (No, that's never happened to us.
// Yet.)
func (s Stage) IsValid() bool {
	switch s {
	case StageStart, StagePRs, StageIssues, StageDone:
		return true
	default:
		return false
	}
}

// GithubCheckpoint is the persisted state of an in-flight ingest run.
//
//   - Stage: where the orchestrator is in its linear walk.
//   - RepoIDsRemaining: full-names of repos yet to fetch (popped front-to-back).
//   - CurrentRepoID / CurrentRepoFullName: the repo currently being
//     paginated. Both nil means "no repo in flight" (Stage = DONE or
//     between repo transitions).
//   - CurrPage: the next page number to fetch in the current repo.
//   - LastSeenUpdatedAt: optimisation for the time-window early-break;
//     resume can short-circuit pages where every PR is older than this.
//
// We embed interfaces.AnyCheckpoint to satisfy the Checkpoint marker —
// the unexported isCheckpoint() method on Checkpoint can only be
// satisfied by types in the interfaces package itself, so embedding is
// the cross-package conformance pattern. AnyCheckpoint contributes the
// HasMore field which we leave at its zero value (the connector signals
// completion via Stage = DONE rather than HasMore).
type GithubCheckpoint struct {
	interfaces.AnyCheckpoint

	Stage               Stage      `json:"stage"`
	RepoIDsRemaining    []string   `json:"repo_full_names_remaining,omitempty"`
	CurrentRepoID       *int64     `json:"current_repo_id,omitempty"`
	CurrentRepoFullName *string    `json:"current_repo_full_name,omitempty"`
	CurrPage            int        `json:"curr_page"`
	LastSeenUpdatedAt   *time.Time `json:"last_seen_updated_at,omitempty"`
}

// dummyCheckpoint is the zero-value used for a fresh "from-beginning"
// run (Stage = START). The orchestrator transitions to PRS / ISSUES /
// DONE on first call.
func dummyCheckpoint() GithubCheckpoint {
	return GithubCheckpoint{Stage: StageStart, CurrPage: 1}
}

// unmarshalCheckpoint parses persisted bytes back into GithubCheckpoint.
// Returns an error on malformed JSON or unknown Stage value — both
// conditions the scheduler should escalate to a fresh-from-scratch
// re-run rather than silently advancing into an unknown state.
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
