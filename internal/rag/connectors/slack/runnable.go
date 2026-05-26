package slack

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// Run adapts the generic LoadFromCheckpoint surface to the non-generic
// RunnableCheckpointed interface expected by the ingest task handler.
func (c *SlackConnector) Run(
	ctx context.Context,
	src interfaces.Source,
	checkpointJSON json.RawMessage,
	start, end time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	cp, err := c.UnmarshalCheckpoint(checkpointJSON)
	if err != nil {
		return nil, err
	}
	return c.LoadFromCheckpoint(ctx, src, cp, start, end)
}

// FinalCheckpoint returns the serialized checkpoint at the end of a run.
func (c *SlackConnector) FinalCheckpoint() (json.RawMessage, error) {
	cp := c.finalCp.Load()
	if cp == nil {
		return nil, nil
	}
	return json.Marshal(*cp)
}

// marshalCP is a helper used internally to serialize any SlackCheckpoint.
func marshalCP(cp SlackCheckpoint) (json.RawMessage, error) {
	return json.Marshal(cp)
}
