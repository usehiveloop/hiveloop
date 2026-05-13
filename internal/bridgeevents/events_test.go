package bridgeevents

import "testing"

func TestNormalizeEventType_LegacyAliases(t *testing.T) {
	cases := map[string]string{
		"ConversationEnded": EventConversationEnded,
		"AgentError":        EventAgentError,
		"tool_call_start":   EventToolCallStarted,
		"tool_call_result":  EventToolCallCompleted,
		"response_chunk":    EventResponseChunk,
	}

	for input, want := range cases {
		if got := NormalizeEventType(input); got != want {
			t.Fatalf("NormalizeEventType(%q) = %q, want %q", input, got, want)
		}
	}
}
