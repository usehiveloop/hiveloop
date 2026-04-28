package website

import (
	"net/url"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

// canonicalURL normalises a URL so re-crawls hit the same qdrant point
// id. Lower-cases scheme + host, drops fragments, drops a trailing slash
// on non-root paths, leaves the query untouched.
func canonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String()
}

func responseToDocument(r spider.Response) interfaces.Document {
	docID := canonicalURL(r.URL)
	return interfaces.Document{
		DocID:      docID,
		SemanticID: r.URL,
		Link:       r.URL,
		Sections:   []interfaces.Section{{Text: r.Content}},
		IsPublic:   true,
	}
}
