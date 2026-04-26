// Pagination via GitHub's RFC 5988 Link header.
//
// GitHub returns a Link response header on paginated endpoints with
// rels {first, prev, next, last}; we only need `next`. Format:
//
//	Link: <https://api.github.com/repos/x/y/pulls?page=2&per_page=100>; rel="next",
//	      <https://api.github.com/repos/x/y/pulls?page=12&per_page=100>; rel="last"
//
// PyGithub hides this; in our world we parse it explicitly because the
// pagination cursor lives in the response, not in our request shape.
//
// Onyx analog: connector.py:166-226 — the offset-pagination branch builds
// page numbers manually. We do the same, but driven by the server's
// `next` link rather than a client-side counter, so we don't have to
// know the total ahead of time and we naturally stop when the server
// stops emitting `next`.
package github

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// nextPageNumber parses the Link header from a GitHub response and
// returns the integer page number for the rel="next" link, plus a
// boolean indicating whether such a link exists.
//
// When no Link header is present (single-page responses) or no `next`
// rel is set (last page), returns (0, false). The caller treats either
// as "stop iterating".
//
// This function tolerates whitespace and rel ordering variations — the
// only invariants are (a) angle-bracketed URL first per Link entry and
// (b) `page=N` somewhere in that URL's query.
func nextPageNumber(h http.Header) (int, bool) {
	link := h.Get("Link")
	if link == "" {
		return 0, false
	}

	// Each Link entry is comma-separated; we walk them looking for the
	// rel="next" marker. Multiple rels per entry are legal but rare;
	// `strings.Contains` is deliberately loose to absorb both quoting
	// styles GitHub has shipped over the years (rel="next" vs rel=next).
	for _, entry := range strings.Split(link, ",") {
		entry = strings.TrimSpace(entry)
		if !strings.Contains(entry, `rel="next"`) && !strings.Contains(entry, `rel=next`) {
			continue
		}

		// Pull the URL out of the angle brackets — RFC 5988 mandates this
		// shape, and GitHub never deviates. If something else has shipped
		// us a malformed entry we fall through to the (0, false) return.
		open := strings.Index(entry, "<")
		close := strings.Index(entry, ">")
		if open < 0 || close <= open {
			continue
		}
		raw := entry[open+1 : close]
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		page := u.Query().Get("page")
		if page == "" {
			continue
		}
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 {
			continue
		}
		return n, true
	}
	return 0, false
}
