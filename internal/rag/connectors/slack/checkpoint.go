package slack

import (
	"encoding/json"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

type Stage string

const (
	StageStart Stage = "START"
	StageDone  Stage = "DONE"
)

func (s Stage) IsValid() bool {
	return s == StageStart || s == StageDone
}

type Checkpoint struct {
	interfaces.AnyCheckpoint
	Stage Stage `json:"stage"`
}

func dummyCheckpoint() Checkpoint {
	return Checkpoint{Stage: StageStart}
}

func unmarshalCheckpoint(raw json.RawMessage) (Checkpoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return dummyCheckpoint(), nil
	}
	var cp Checkpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return Checkpoint{}, fmt.Errorf("slack: parse checkpoint: %w", err)
	}
	if cp.Stage == "" {
		cp.Stage = StageStart
	}
	if !cp.Stage.IsValid() {
		return Checkpoint{}, fmt.Errorf("slack: invalid stage %q", cp.Stage)
	}
	return cp, nil
}
