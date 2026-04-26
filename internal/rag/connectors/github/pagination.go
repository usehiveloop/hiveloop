package github

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// nextPageNumber returns (0, false) for single-page responses or the
// last page. Tolerates both rel="next" and rel=next quoting styles GitHub
// has shipped over the years.
func nextPageNumber(h http.Header) (int, bool) {
	return relPageNumber(h, "next")
}

// lastPageNumber extracts the rel="last" page number — used for cheap
// total-count estimation by paging at per_page=1, where the last page
// number equals the total item count.
func lastPageNumber(h http.Header) (int, bool) {
	return relPageNumber(h, "last")
}

func relPageNumber(h http.Header, rel string) (int, bool) {
	link := h.Get("Link")
	if link == "" {
		return 0, false
	}
	wantQuoted := `rel="` + rel + `"`
	wantBare := `rel=` + rel

	for _, entry := range strings.Split(link, ",") {
		entry = strings.TrimSpace(entry)
		if !strings.Contains(entry, wantQuoted) && !strings.Contains(entry, wantBare) {
			continue
		}

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
