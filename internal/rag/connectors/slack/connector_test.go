package slack

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func TestLoadConfigValidatesHistoryDays(t *testing.T) {
	_, err := LoadConfig(json.RawMessage(`{"agent_profile_id":"550e8400-e29b-41d4-a716-446655440000","history_days":30}`))
	if err == nil {
		t.Fatalf("expected history_days validation error")
	}
	cfg, err := LoadConfig(json.RawMessage(`{"agent_profile_id":"550e8400-e29b-41d4-a716-446655440000"}`))
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.HistoryDays != 90 || !cfg.IncludePublicChannels || !cfg.IncludeJoinedPrivateChannels {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestConnectorIngestsPublicChannelsAndMemberPrivateChannelsWithoutJoining(t *testing.T) {
	api := &fakeAPI{
		channels: map[string][]channel{
			"public_channel": {
				{ID: "C1", Name: "general", IsMember: true},
				{ID: "C2", Name: "random", IsMember: false},
			},
			"private_channel": {
				{ID: "G1", Name: "leadership", IsPrivate: true, IsMember: true},
				{ID: "G2", Name: "finance", IsPrivate: true, IsMember: false},
			},
		},
		history: map[string][]message{
			"C1": {
				{User: "U1", Text: "Deploys must include rollback notes.", Timestamp: "1778457600.000000", ReplyCount: 1},
			},
			"C2": {
				{User: "U4", Text: "Public non-member channels are org knowledge.", Timestamp: "1778458200.000000"},
			},
			"G1": {
				{User: "U2", Text: "Hiring plan moves to next week.", Timestamp: "1778461200.000000"},
			},
		},
		replies: map[string][]message{
			"C1:1778457600.000000": {
				{User: "U1", Text: "Deploys must include rollback notes.", Timestamp: "1778457600.000000"},
				{User: "U3", Text: "Agreed. Add the incident link too.", Timestamp: "1778457660.000000"},
			},
		},
	}
	conn := NewConnector(Config{
		AgentProfileID:               "550e8400-e29b-41d4-a716-446655440000",
		HistoryDays:                  90,
		IncludePublicChannels:        true,
		IncludeJoinedPrivateChannels: true,
	}, api, slack.Identity{TeamURL: "https://acme.slack.com", TeamID: "T1"})

	start := time.Date(2026, 5, 11, 12, 30, 0, 0, time.UTC)
	stream, err := conn.LoadFromCheckpoint(context.Background(), &testSource{}, dummyCheckpoint(), start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var docs []interfaces.Document
	for item := range stream {
		if item.Failure != nil {
			t.Fatalf("unexpected failure: %#v", item.Failure)
		}
		docs = append(docs, *item.Doc)
	}
	if len(docs) != 3 {
		t.Fatalf("expected public channels plus member private channel, got %d: %#v", len(docs), docs)
	}
	if api.joinCalled {
		t.Fatalf("connector must not call join")
	}
	if api.oldest["C1"] != "1778457600.000000" {
		t.Fatalf("expected full UTC day oldest, got %q", api.oldest["C1"])
	}
	if docs[0].DocID != "slack:C1:2026-05-11" {
		t.Fatalf("doc id = %q", docs[0].DocID)
	}
	transcript := docs[0].Sections[0].Text
	if !containsAll(transcript, "Deploys must include rollback notes.", "thread reply", "incident link") {
		t.Fatalf("thread transcript missing expected content: %s", transcript)
	}
	if docs[1].DocID != "slack:C2:2026-05-11" {
		t.Fatalf("public non-member channel was not indexed: %#v", docs)
	}
	if docs[2].Metadata["visibility"] != "private_channel_org_visible" {
		t.Fatalf("private visibility metadata = %#v", docs[2].Metadata)
	}
}

func TestConnectorRediscoversNewlyAccessibleChannelsOnNextRun(t *testing.T) {
	api := &fakeAPI{
		channels: map[string][]channel{
			"public_channel": {{ID: "C1", Name: "general", IsMember: true}},
		},
		history: map[string][]message{
			"C1": {{User: "U1", Text: "First channel context.", Timestamp: "1778457600.000000"}},
		},
	}
	conn := NewConnector(Config{
		AgentProfileID:        "550e8400-e29b-41d4-a716-446655440000",
		HistoryDays:           90,
		IncludePublicChannels: true,
	}, api, slack.Identity{TeamURL: "https://acme.slack.com", TeamID: "T1"})

	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	first := drainDocs(t, conn, start, start.Add(time.Hour))
	if len(first) != 1 || first[0].DocID != "slack:C1:2026-05-11" {
		t.Fatalf("first run docs = %#v", first)
	}

	api.channels["public_channel"] = append(api.channels["public_channel"], channel{ID: "C3", Name: "product", IsMember: true})
	api.history["C3"] = []message{{User: "U2", Text: "Newly invited channel context.", Timestamp: "1778457900.000000"}}

	second := drainDocs(t, conn, start, start.Add(time.Hour))
	if len(second) != 2 {
		t.Fatalf("second run should discover newly accessible channel, got %#v", second)
	}
	if second[1].DocID != "slack:C3:2026-05-11" {
		t.Fatalf("new channel doc id = %q", second[1].DocID)
	}
}

func TestConnectorSkipsBotDominatedChannels(t *testing.T) {
	api := &fakeAPI{
		channels: map[string][]channel{
			"public_channel": {
				{ID: "C1", Name: "humans"},
				{ID: "C2", Name: "deploy-bot"},
			},
		},
		history: map[string][]message{
			"C1": {
				{User: "U1", Text: "Customer escalation policy changed.", Timestamp: "1778457600.000000"},
				{User: "U2", Text: "Support should loop in product earlier.", Timestamp: "1778457660.000000"},
			},
			"C2": {
				{Username: "ci-bot", Text: "Build passed.", Timestamp: "1778457600.000000", SubType: "bot_message"},
				{Username: "deploy-bot", Text: "Deploy started.", Timestamp: "1778457660.000000", SubType: "bot_message"},
				{User: "U1", Text: "Ack.", Timestamp: "1778457720.000000"},
			},
		},
	}
	conn := NewConnector(Config{
		AgentProfileID:        "550e8400-e29b-41d4-a716-446655440000",
		HistoryDays:           90,
		IncludePublicChannels: true,
	}, api, slack.Identity{TeamURL: "https://acme.slack.com", TeamID: "T1"})

	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	docs := drainDocs(t, conn, start, start.Add(time.Hour))
	if len(docs) != 1 {
		t.Fatalf("expected bot-dominated channel to be skipped, got %#v", docs)
	}
	if docs[0].DocID != "slack:C1:2026-05-11" {
		t.Fatalf("unexpected doc indexed: %#v", docs[0])
	}
}

func TestConnectorSkipsPublicChannelsSlackRefusesToRead(t *testing.T) {
	api := &fakeAPI{
		channels: map[string][]channel{
			"public_channel": {
				{ID: "C1", Name: "joined", IsMember: true},
				{ID: "C2", Name: "not-joined", IsMember: false},
			},
		},
		history: map[string][]message{
			"C1": {{User: "U1", Text: "Joined public channel context.", Timestamp: "1778457600.000000"}},
		},
		historyErr: map[string]error{
			"C2": errors.New("not_in_channel"),
		},
	}
	conn := NewConnector(Config{
		AgentProfileID:        "550e8400-e29b-41d4-a716-446655440000",
		HistoryDays:           90,
		IncludePublicChannels: true,
	}, api, slack.Identity{TeamURL: "https://acme.slack.com", TeamID: "T1"})

	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	docs := drainDocs(t, conn, start, start.Add(time.Hour))
	if len(docs) != 1 {
		t.Fatalf("expected unreadable public channel to be skipped, got %#v", docs)
	}
	if docs[0].DocID != "slack:C1:2026-05-11" {
		t.Fatalf("unexpected doc indexed: %#v", docs[0])
	}
}

type fakeAPI struct {
	channels   map[string][]channel
	history    map[string][]message
	historyErr map[string]error
	replies    map[string][]message
	oldest     map[string]string
	joinCalled bool
}

func (f *fakeAPI) ListConversations(_ context.Context, conversationTypes []string, cursor string) ([]channel, string, error) {
	if cursor != "" {
		return nil, "", nil
	}
	return f.channels[conversationTypes[0]], "", nil
}

func (f *fakeAPI) History(_ context.Context, channelID, oldest, _, cursor string) ([]message, string, error) {
	if cursor != "" {
		return nil, "", nil
	}
	if err := f.historyErr[channelID]; err != nil {
		return nil, "", err
	}
	if f.oldest == nil {
		f.oldest = map[string]string{}
	}
	f.oldest[channelID] = oldest
	return f.history[channelID], "", nil
}

func (f *fakeAPI) Replies(_ context.Context, channelID, threadTS, _, _, cursor string) ([]message, string, error) {
	if cursor != "" {
		return nil, "", nil
	}
	return f.replies[channelID+":"+threadTS], "", nil
}

type testSource struct{}

func (s *testSource) SourceID() string        { return "source" }
func (s *testSource) OrgID() string           { return "org" }
func (s *testSource) SourceKind() string      { return Kind }
func (s *testSource) Config() json.RawMessage { return json.RawMessage(`{}`) }

func drainDocs(t *testing.T, conn *Connector, start, end time.Time) []interfaces.Document {
	t.Helper()
	stream, err := conn.LoadFromCheckpoint(context.Background(), &testSource{}, dummyCheckpoint(), start, end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var docs []interfaces.Document
	for item := range stream {
		if item.Failure != nil {
			t.Fatalf("unexpected failure: %#v", item.Failure)
		}
		docs = append(docs, *item.Doc)
	}
	return docs
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
