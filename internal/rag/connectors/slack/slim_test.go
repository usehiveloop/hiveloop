package slack

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSlimList_Basic(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setChannels(makeChannel("C1", "general", true, false))
	fake.setHistory("C1", []SlackMessage{
		{Type: "message", User: "U1", Text: "Msg1", TS: "1000.000002"},
		{Type: "message", User: "U2", Text: "Msg2", TS: "1000.000001"},
	}, false)

	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"
	c.memberChannels = fake.channels

	ch, err := c.ListAllSlim(context.Background(), &fixtureSource{cfg: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}
	slims, fails := drainSlims(t, ch)
	if len(fails) > 0 {
		t.Errorf("unexpected failures: %d", len(fails))
	}
	if len(slims) != 2 {
		t.Fatalf("expected 2 slim docs, got %d", len(slims))
	}
	for _, s := range slims {
		if !strings.Contains(s.DocID, "C1__") {
			t.Errorf("unexpected doc ID: %s", s.DocID)
		}
	}
}
