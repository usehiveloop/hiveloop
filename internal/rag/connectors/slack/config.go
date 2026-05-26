package slack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// SlackConfig is the connector-specific configuration stored in
// RAGSource.ConfigValue (JSONB). The user selects which channels to
// index and whether bot messages are included.
type SlackConfig struct {
	ChannelNames        []string `json:"channel_names,omitempty"`
	IncludeBotMessages  bool     `json:"include_bot_messages"`
	ChannelRegexEnabled bool     `json:"channel_regex_enabled"`
}

func LoadConfig(raw json.RawMessage) (SlackConfig, error) {
	cfg := SlackConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return SlackConfig{}, fmt.Errorf("slack: parse config: %w", err)
		}
	}
	cfg.ChannelNames = normaliseChannelList(cfg.ChannelNames)
	return cfg, nil
}

func normaliseChannelList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	dedup := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, name := range in {
		name = strings.TrimSpace(name)
		name = strings.TrimPrefix(name, "#")
		if name == "" {
			continue
		}
		if _, ok := dedup[name]; ok {
			continue
		}
		dedup[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// channelIsAllowed returns true if the channel should be indexed.
// When no channel filter is configured, all member channels are included.
func channelIsAllowed(channel SlackChannel, names []string, regexEnabled bool) bool {
	if len(names) == 0 {
		return true
	}
	if regexEnabled {
		return channelMatchesRegex(channel, names)
	}
	return channelInList(channel, names)
}

func channelInList(channel SlackChannel, names []string) bool {
	for _, n := range names {
		if channel.Name == n {
			return true
		}
	}
	return false
}

func channelMatchesRegex(channel SlackChannel, patterns []string) bool {
	for _, p := range patterns {
		if match, _ := wildcardMatch(p, channel.Name); match {
			return true
		}
	}
	return false
}

func wildcardMatch(pattern, s string) (bool, error) {
	re, err := compileGlob(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

func compileGlob(pat string) (*regexp.Regexp, error) {
	raw := strings.ReplaceAll(regexp.QuoteMeta(pat), `\*`, ".*")
	return regexp.Compile(`^` + raw + `$`)
}
