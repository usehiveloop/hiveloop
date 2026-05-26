package slack

import "testing"

func TestTextCleaner_SpecialMentions(t *testing.T) {
	cleaner := newTextCleaner(nil, nil)
	tests := []struct{ input, want string }{
		{"Hello <!channel>", "Hello @channel"},
		{"Hey <!here>", "Hey @here"},
		{`<@U123> told <!everyone> to <!channel>`, `<@U123> told @everyone to @channel`},
	}
	for _, tc := range tests {
		got := cleaner.replaceSpecialMentions(tc.input)
		if got != tc.want {
			t.Errorf("replaceSpecialMentions(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTextCleaner_ChannelRefs(t *testing.T) {
	cleaner := newTextCleaner(nil, nil)
	input := "See <#C123|general> and <#C456|random>"
	want := "See #general and #random"
	if got := cleaner.replaceChannelRefs(input); got != want {
		t.Errorf("replaceChannelRefs = %q, want %q", got, want)
	}
}

func TestTextCleaner_SubteamRefs(t *testing.T) {
	cleaner := newTextCleaner(nil, nil)
	input := "Tag <!subteam^S123|@engineering> for help"
	want := "Tag @engineering for help"
	if got := cleaner.replaceSubteamRefs(input); got != want {
		t.Errorf("replaceSubteamRefs = %q, want %q", got, want)
	}
}

func TestTextCleaner_ChannelRefWithoutPipe(t *testing.T) {
	cleaner := newTextCleaner(nil, nil)
	input := "<#C123>"
	if got := cleaner.replaceChannelRefs(input); got != input {
		t.Errorf("channel ref without pipe should stay: got %q", got)
	}
}
