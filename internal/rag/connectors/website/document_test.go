package website

import "testing"

func TestCanonicalURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"HTTPS://Example.com/Docs/", "https://example.com/Docs"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com/page#section-1", "https://example.com/page"},
		{"https://example.com/q?a=1&b=2", "https://example.com/q?a=1&b=2"},
		{"  https://example.com/x  ", "https://example.com/x"},
	}
	for _, c := range cases {
		if got := canonicalURL(c.in); got != c.want {
			t.Errorf("canonicalURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCanonicalURL_StableUnderRecrawl(t *testing.T) {
	a := canonicalURL("https://example.com/Docs/Guide/")
	b := canonicalURL("HTTPS://example.com/Docs/Guide/#footer")
	if a != b {
		t.Fatalf("same logical URL → different canonical:\n  %q\n  %q", a, b)
	}
}
