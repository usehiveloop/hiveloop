package github

import (
	"context"
	"encoding/json"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

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

func (c *GithubConnector) FinalCheckpoint() (json.RawMessage, error) {
	cp := c.finalCp.Load()
	if cp == nil {
		return nil, nil
	}
	return json.Marshal(*cp)
}
