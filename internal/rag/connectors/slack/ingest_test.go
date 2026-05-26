package slack

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestIngest_ChannelNotFound_ClosesGracefully(t *testing.T) {
	fake := newFakeSlackAPI()
	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	cp := dummyCheckpoint()
	ch, err := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, cp, time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docs, _ := drainDocs(t, ch)
	if len(docs) != 0 {
		t.Errorf("expected 0 docs for empty workspace, got %d", len(docs))
	}
}

func TestIngest_SingleChannel_BasicFlow(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setUser("U2", "Bob", "bob@test.com")

	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U2", Text: "Good morning", TS: "1000.000003"},
		{Type: "message", User: "U1", Text: "Hello world", TS: "1000.000002"},
		{Type: "message", User: "U1", Text: "Thread starter", TS: "1000.000001", ThreadTS: "1000.000001"},
	}, false)
	fake.setReplies("C1", "1000.000001", []SlackMessage{
		{Type: "message", User: "U1", Text: "Thread starter", TS: "1000.000001", ThreadTS: "1000.000001"},
		{Type: "message", User: "U2", Text: "Thread reply", TS: "1000.000004", ThreadTS: "1000.000001"},
	})

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	cp := dummyCheckpoint()
	ch, err := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, cp, time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docs, fails := drainDocs(t, ch)
	if len(fails) > 0 {
		t.Errorf("unexpected failures: %d", len(fails))
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(docs))
	}
}

func TestIngest_FiltersBotsByDefault(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Real message", TS: "1000.000002"},
		{Type: "message", BotID: "B1", Text: "Bot message", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, _ := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, dummyCheckpoint(), time.Time{}, time.Now())
	docs, _ := drainDocs(t, ch)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc (bot filtered), got %d", len(docs))
	}
}

func TestIngest_IncludesBotsWhenConfigured(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Real message", TS: "1000.000002"},
		{Type: "message", BotID: "B1", BotProfile: &BotProfile{Name: "TestBot"}, Text: "Bot message", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{IncludeBotMessages: true}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, _ := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, dummyCheckpoint(), time.Time{}, time.Now())
	docs, _ := drainDocs(t, ch)
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs (bot included), got %d", len(docs))
	}
}

func TestIngest_CheckpointSave(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Message 2", TS: "1000.000002"},
		{Type: "message", User: "U1", Text: "Message 1", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, _ := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, dummyCheckpoint(), time.Time{}, time.Now())
	drainDocs(t, ch)

	finalCp := c.finalCp.Load()
	if finalCp == nil {
		t.Fatal("finalCp should not be nil")
	}
	if finalCp.ChannelCompletionMap["C1"] == "" {
		t.Error("checkpoint should have recorded channel progress")
	}
}

func TestIngest_MultipleChannels(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(
		makeChannel("C1", "general", true, false),
		makeChannel("C2", "random", true, false),
	)
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "C1 msg", TS: "1000.000001"},
	}, false)
	fake.setHistory("C2", []SlackMessage{
		{Type: "message", User: "U1", Text: "C2 msg", TS: "1000.000002"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	ch, _ := c.LoadFromCheckpoint(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)}, dummyCheckpoint(), time.Time{}, time.Now())
	docs, _ := drainDocs(t, ch)
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs across channels, got %d", len(docs))
	}
}
