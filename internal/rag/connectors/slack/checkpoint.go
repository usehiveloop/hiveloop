package slack

import (
	"encoding/json"
	"fmt"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// SlackCheckpoint stores incremental progress so the connector can
// resume across Asynq retries and worker restarts. Slack returns
// messages newest-to-oldest, so channel_completion_map tracks the
// oldest timestamp we've processed per channel.
//
// Ported from Onyx's SlackCheckpoint at
// backend/onyx/connectors/slack/connector.py:79-91.
type SlackCheckpoint struct {
	interfaces.AnyCheckpoint

	// ChannelIDs is populated once at startup from conversations.list
	// filtered to channels where is_member=true.
	ChannelIDs []string `json:"channel_ids,omitempty"`

	// ChannelCompletionMap tracks each channel's progress.
	// key = channel ID (e.g. "C012AB3CD"), value = oldest ts seen.
	ChannelCompletionMap map[string]string `json:"channel_completion_map,omitempty"`

	// CurrentChannelID is the channel currently being processed.
	CurrentChannelID string `json:"current_channel_id,omitempty"`

	// CurrentChannelName is the human-readable name for logging.
	CurrentChannelName string `json:"current_channel_name,omitempty"`

	// CurrentChannelIsPrivate stores the privacy flag of
	// the current channel for permission resolution.
	CurrentChannelIsPrivate bool `json:"current_channel_is_private"`

	// SeenThreadTS tracks thread timestamps already processed so we
	// don't re-fetch threads when the same thread_ts appears in
	// later message batches.
	SeenThreadTS []string `json:"seen_thread_ts,omitempty"`

	// WorkspaceURL is cached from auth.test so we don't re-fetch it.
	WorkspaceURL string `json:"workspace_url,omitempty"`
}

func dummyCheckpoint() SlackCheckpoint {
	return SlackCheckpoint{
		AnyCheckpoint: interfaces.AnyCheckpoint{HasMore: true},
	}
}

func unmarshalCheckpoint(raw json.RawMessage) (SlackCheckpoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		cp := dummyCheckpoint()
		cp.ChannelCompletionMap = make(map[string]string)
		return cp, nil
	}
	var cp SlackCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return SlackCheckpoint{}, fmt.Errorf("slack: parse checkpoint: %w", err)
	}
	if cp.ChannelCompletionMap == nil {
		cp.ChannelCompletionMap = make(map[string]string)
	}
	return cp, nil
}
