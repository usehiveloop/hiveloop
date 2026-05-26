package slack

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ============================================================
// Registration tests
// ============================================================

func TestRegistration_SlackKindResolves(t *testing.T) {
	factory, err := interfaces.Lookup(Kind)
	if err != nil {
		t.Fatalf("interfaces.Lookup(%q): %v", Kind, err)
	}
	if factory == nil {
		t.Fatal("Lookup returned nil factory")
	}
}

func TestRegistration_SlackBuildsConnector(t *testing.T) {
	factory, err := interfaces.Lookup(Kind)
	if err != nil {
		t.Fatal(err)
	}
	src := &fixtureSource{cfg: json.RawMessage(`{}`)}
	conn, err := factory(src, interfaces.BuildDeps{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if conn == nil {
		t.Fatal("factory returned nil connector")
	}
	if conn.Kind() != Kind {
		t.Fatalf("Kind() = %q, want %q", conn.Kind(), Kind)
	}
	if _, ok := conn.(*SlackConnector); !ok {
		t.Fatalf("factory returned %T, want *SlackConnector", conn)
	}
}

// ============================================================
// Config tests
// ============================================================

func TestConfig_EmptyConfig(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IncludeBotMessages {
		t.Error("IncludeBotMessages should default to false")
	}
	if cfg.ChannelRegexEnabled {
		t.Error("ChannelRegexEnabled should default to false")
	}
	if cfg.ChannelNames != nil {
		t.Error("ChannelNames should be nil for empty config")
	}
}

func TestConfig_ChannelNormalisation(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{"channel_names":["  #general  ","random",""]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"general", "random"}
	if !reflect.DeepEqual(cfg.ChannelNames, want) {
		t.Fatalf("ChannelNames = %v, want %v", cfg.ChannelNames, want)
	}
}

func TestConfig_DuplicateChannelsDedupd(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{"channel_names":["general","general","random","#general"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"general", "random"}
	if !reflect.DeepEqual(cfg.ChannelNames, want) {
		t.Fatalf("ChannelNames = %v, want %v", cfg.ChannelNames, want)
	}
}

func TestConfig_IncludeBotMessages(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{"include_bot_messages":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IncludeBotMessages {
		t.Error("IncludeBotMessages should be true")
	}
}

func TestChannelIsAllowed_NoFilter(t *testing.T) {
	ch := makeChannel("C1", "general", true, false)
	if !channelIsAllowed(ch, nil, false) {
		t.Error("channel should be allowed with no filter")
	}
}

func TestChannelIsAllowed_ByName(t *testing.T) {
	ch := makeChannel("C1", "general", true, false)
	if !channelIsAllowed(ch, []string{"general"}, false) {
		t.Error("channel should be allowed by name match")
	}
	if channelIsAllowed(ch, []string{"random"}, false) {
		t.Error("channel should NOT be allowed by non-matching name")
	}
}

func TestChannelIsAllowed_Regex(t *testing.T) {
	ch := makeChannel("C1", "eng-team", true, false)
	if !channelIsAllowed(ch, []string{"eng-*"}, true) {
		t.Error("channel should match eng-* glob")
	}
	if channelIsAllowed(ch, []string{"ops-*"}, true) {
		t.Error("channel should NOT match ops-* glob")
	}
}
