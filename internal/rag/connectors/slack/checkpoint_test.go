package slack

import (
	"encoding/json"
	"testing"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func TestCheckpoint_RoundTrip(t *testing.T) {
	cp := SlackCheckpoint{
		AnyCheckpoint: interfaces.AnyCheckpoint{HasMore: true},
		ChannelIDs:    []string{"C1", "C2"},
		ChannelCompletionMap: map[string]string{"C1": "1512085950.000216"},
		CurrentChannelID:     "C2",
		SeenThreadTS:         []string{"1512085950.000216"},
		WorkspaceURL:         "https://test.slack.com/",
	}
	raw, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalCheckpoint(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ChannelIDs) != 2 {
		t.Errorf("ChannelIDs len = %d, want 2", len(got.ChannelIDs))
	}
	if got.CurrentChannelID != "C2" {
		t.Errorf("CurrentChannelID = %q, want C2", got.CurrentChannelID)
	}
	if got.ChannelCompletionMap["C1"] != "1512085950.000216" {
		t.Error("ChannelCompletionMap not preserved")
	}
}

func TestCheckpoint_UnmarshalNil(t *testing.T) {
	cp, err := unmarshalCheckpoint(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cp.ChannelIDs != nil {
		t.Error("nil checkpoint should have nil ChannelIDs")
	}
	if cp.ChannelCompletionMap == nil {
		t.Error("nil checkpoint should initialise ChannelCompletionMap")
	}
}

func TestCheckpoint_UnmarshalEmpty(t *testing.T) {
	cp, err := unmarshalCheckpoint(json.RawMessage(`{"channel_ids":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if cp.ChannelCompletionMap == nil {
		t.Error("empty checkpoint should initialise ChannelCompletionMap")
	}
}
