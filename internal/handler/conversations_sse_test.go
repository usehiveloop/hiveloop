package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrimSSEEnvelope_StripsConversationIDAndAgentID(t *testing.T) {
	in := json.RawMessage(`{"event_id":"e1","agent_id":"a1","conversation_id":"c1","data":{"x":1}}`)
	out := trimSSEEnvelope(in)

	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("invalid JSON out: %v (%s)", err, out)
	}
	if _, ok := obj["conversation_id"]; ok {
		t.Errorf("conversation_id should be stripped, got %s", out)
	}
	if _, ok := obj["agent_id"]; ok {
		t.Errorf("agent_id should be stripped, got %s", out)
	}
	if obj["event_id"] != "e1" {
		t.Errorf("event_id missing or wrong: %s", out)
	}
	if _, ok := obj["data"]; !ok {
		t.Errorf("data should be preserved, got %s", out)
	}
}

func TestTrimSSEEnvelope_EmptyInput(t *testing.T) {
	if got := trimSSEEnvelope(nil); len(got) != 0 {
		t.Errorf("nil input should return empty, got %s", got)
	}
	if got := trimSSEEnvelope(json.RawMessage{}); len(got) != 0 {
		t.Errorf("empty input should return empty, got %s", got)
	}
}

func TestTrimSSEEnvelope_InvalidJSONReturnsOriginal(t *testing.T) {
	in := json.RawMessage(`{not valid`)
	got := trimSSEEnvelope(in)
	if string(got) != string(in) {
		t.Errorf("on parse error should return original bytes; got %s", got)
	}
}

func TestTrimSSEEnvelope_NonObjectReturnsOriginal(t *testing.T) {
	in := json.RawMessage(`[1,2,3]`)
	got := trimSSEEnvelope(in)
	if string(got) != string(in) {
		t.Errorf("array input should be returned unchanged; got %s", got)
	}
}

func TestTrimSSEEnvelope_PreservesOtherFields(t *testing.T) {
	in := json.RawMessage(`{"conversation_id":"c1","agent_id":"a1","timestamp":"2025-01-01T00:00:00Z","sequence_number":42,"event_type":"msg","data":{"content":"hi"}}`)
	out := trimSSEEnvelope(in)
	if strings.Contains(string(out), "conversation_id") || strings.Contains(string(out), "agent_id") {
		t.Errorf("stripped fields still present: %s", out)
	}
	for _, field := range []string{"timestamp", "sequence_number", "event_type", "data"} {
		if !strings.Contains(string(out), field) {
			t.Errorf("field %q missing after trim: %s", field, out)
		}
	}
}

func TestStrconvAtoiPositive(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"1", 1, false},
		{"200", 200, false},
		{"999", 999, false},
		{"0", 0, true},
		{"-5", 0, true},
		{"", 0, true},
		{"abc", 0, true},
		{"12a", 0, true},
	}
	for _, c := range cases {
		got, err := strconvAtoiPositive(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: wantErr=%v got err=%v", c.in, c.wantErr, err)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("%q: got %d, want %d", c.in, got, c.want)
		}
	}
}
