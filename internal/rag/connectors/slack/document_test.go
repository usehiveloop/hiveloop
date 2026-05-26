package slack

import (
	"context"
	"strings"
	"testing"
)

func TestDocID(t *testing.T) {
	id := docID("C123", "1512085950.000216")
	if id != "C123__1512085950.000216" {
		t.Errorf("docID = %q, want C123__1512085950.000216", id)
	}
}

func TestDocIDFromMessage_ThreadParent(t *testing.T) {
	msg := SlackMessage{TS: "1512085950.000217", ThreadTS: "1512085950.000216"}
	id := docIDFromMessage("C123", msg)
	if id != "C123__1512085950.000216" {
		t.Errorf("thread parent docID = %q, want C123__1512085950.000216", id)
	}
}

func TestDocIDFromMessage_Standalone(t *testing.T) {
	msg := SlackMessage{TS: "1512085950.000216"}
	id := docIDFromMessage("C123", msg)
	if id != "C123__1512085950.000216" {
		t.Errorf("standalone docID = %q, want C123__1512085950.000216", id)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 50, "short"},
		{"a very long message that goes beyond fifty characters for sure", 50, "a very long message that goes beyond fifty charact..."},
		{"", 10, ""},
		{"exactly ten chars", 17, "exactly ten chars"},
	}
	for _, tc := range tests {
		got := truncate(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestEpochSeconds(t *testing.T) {
	if sec := epochSeconds("1512085950.000216"); sec != 1512085950 {
		t.Errorf("epochSeconds = %d, want 1512085950", sec)
	}
	if sec := epochSeconds("not-a-ts"); sec != 0 {
		t.Errorf("epochSeconds for invalid input = %d, want 0", sec)
	}
}

func TestMessagePermalink(t *testing.T) {
	link := messagePermalink("https://test.slack.com/", "C123", "1512085950.000216")
	want := "https://test.slack.com/archives/C123/p1512085950000216"
	if link != want {
		t.Errorf("messagePermalink = %q, want %q", link, want)
	}
}

func TestThreadPermalink(t *testing.T) {
	link := threadPermalink("https://test.slack.com/", "C123", "1512085950.000217", "1512085950.000216")
	if !strings.Contains(link, "?thread_ts=1512085950.000216") {
		t.Errorf("threadPermalink missing thread_ts: %s", link)
	}
}

func TestThreadToDoc_StandaloneMessage(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setUser("U1", "Alice", "alice@test.com")
	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	channel := makeChannel("C1", "general", true, false)
	messages := []SlackMessage{
		{Type: "message", User: "U1", Text: "Hello world", TS: "1512085950.000216"},
	}
	doc := c.threadToDoc(channel, messages, c.cleaner)
	if doc.DocID != "C1__1512085950.000216" {
		t.Errorf("DocID = %q", doc.DocID)
	}
	if len(doc.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Metadata["Channel"] != "general" {
		t.Errorf("metadata Channel = %q", doc.Metadata["Channel"])
	}
}

func TestThreadToDoc_ThreadedMessages(t *testing.T) {
	fake := newFakeSlackAPI()
	fake.setUser("U1", "Alice", "alice@test.com")
	fake.setUser("U2", "Bob", "bob@test.com")
	c := newConnectorWithAPI(SlackConfig{}, fake)
	c.ctx = context.Background()
	c.workspaceURL = "https://test.slack.com"

	channel := makeChannel("C1", "general", true, false)
	messages := []SlackMessage{
		{Type: "message", User: "U1", Text: "Q: How do I...", TS: "1000.000001", ThreadTS: "1000.000001"},
		{Type: "message", User: "U2", Text: "A: You should...", TS: "1000.000002", ThreadTS: "1000.000001"},
	}
	doc := c.threadToDoc(channel, messages, c.cleaner)
	if doc.DocID != "C1__1000.000001" {
		t.Errorf("DocID = %q, want C1__1000.000001", doc.DocID)
	}
	if len(doc.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(doc.Sections))
	}
}
