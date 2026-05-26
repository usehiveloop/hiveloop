package slack

import "testing"

func TestShouldFilter_BotMessage(t *testing.T) {
	msg := SlackMessage{BotID: "B123", Text: "hello", TS: "1.0", Type: "message"}
	if shouldFilter(msg, false) != "bot" {
		t.Error("bot message should be filtered")
	}
}

func TestShouldFilter_AppMessage(t *testing.T) {
	msg := SlackMessage{AppID: "A123", Text: "hello", TS: "1.0", Type: "message"}
	if shouldFilter(msg, false) != "bot" {
		t.Error("app message should be filtered")
	}
}

func TestShouldFilter_BotIncluded(t *testing.T) {
	msg := SlackMessage{BotID: "B123", BotProfile: &BotProfile{Name: "TestBot"}, Text: "hello", TS: "1.0", Type: "message"}
	if shouldFilter(msg, true) != "" {
		t.Error("bot message should NOT be filtered when includeBots=true")
	}
}

func TestShouldFilter_DisallowedSubtype(t *testing.T) {
	for _, subtype := range []string{"channel_join", "channel_leave", "channel_archive", "channel_name"} {
		msg := SlackMessage{Subtype: subtype, Text: "msg", TS: "1.0", Type: "message"}
		if shouldFilter(msg, false) != "disallowed_subtype" {
			t.Errorf("subtype %s should be filtered", subtype)
		}
	}
}

func TestShouldFilter_ValidUserMessage(t *testing.T) {
	msg := SlackMessage{User: "U123", Text: "hello world", TS: "1512085950.000216", Type: "message"}
	if shouldFilter(msg, false) != "" {
		t.Error("valid user message should NOT be filtered")
	}
}
