package slack

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPermSync_PublicChannel(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Hello", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"
	c.memberChannels = fake.channels

	ch, err := c.SyncDocPermissions(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("SyncDocPermissions: %v", err)
	}
	accesses, _ := drainAccesses(t, ch)
	if len(accesses) != 1 {
		t.Fatalf("expected 1 access record, got %d", len(accesses))
	}
	if !accesses[0].ExternalAccess.IsPublic {
		t.Error("public channel should have IsPublic=true")
	}
}

func TestPermSync_PrivateChannel(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "private-stuff", true, true))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setMembers("C1", []string{"U1"})
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Secret", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"
	c.memberChannels = fake.channels

	ch, _ := c.SyncDocPermissions(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)})
	accesses, _ := drainAccesses(t, ch)
	if len(accesses) != 1 {
		t.Fatalf("expected 1 access record, got %d", len(accesses))
	}
	if accesses[0].ExternalAccess.IsPublic {
		t.Error("private channel should have IsPublic=false")
	}
}

func TestSyncExternalGroups_PrivateChannel(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "private-stuff", true, true))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setUser("U2", "Bob", "bob@test.com")
	fake.setMembers("C1", []string{"U1", "U2"})

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"
	c.memberChannels = fake.channels

	ch, err := c.SyncExternalGroups(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("SyncExternalGroups: %v", err)
	}
	groups, _ := drainGroups(t, ch)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].GroupID != "slack_channel_C1" {
		t.Errorf("GroupID = %q, want slack_channel_C1", groups[0].GroupID)
	}
	if groups[0].DisplayName != "#private-stuff" {
		t.Errorf("DisplayName = %q, want #private-stuff", groups[0].DisplayName)
	}
}

func TestSyncExternalGroups_PublicChannel_NoGroup(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"
	c.memberChannels = fake.channels

	ch, _ := c.SyncExternalGroups(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)})
	groups, _ := drainGroups(t, ch)
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups for public channel, got %d", len(groups))
	}
}
