package github

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// Run is the non-generic adapter the ingest worker drives. It hands the
// raw checkpoint bytes off to UnmarshalCheckpoint, then delegates to
// LoadFromCheckpoint. The connector's run goroutine writes the final
// checkpoint state to c.finalCp when it exits, so FinalCheckpoint can
// return it once the channel closes.
func (c *GithubConnector) Run(
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

// FinalCheckpoint returns the marshaled checkpoint state captured by the
// run goroutine on exit. Returns (nil, nil) when the run never started
// or hasn't finished yet — the worker treats that as "no resumable
// state" and the next attempt will start fresh.
func (c *GithubConnector) FinalCheckpoint() (json.RawMessage, error) {
	cp := c.finalCp.Load()
	if cp == nil {
		return nil, nil
	}
	return json.Marshal(*cp)
}
