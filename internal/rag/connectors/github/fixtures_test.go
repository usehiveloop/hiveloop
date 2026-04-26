package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

type fakeProxy struct {
	mu          sync.Mutex
	pages       map[string][]byte
	defaults    map[string][]byte
	headers     map[string]http.Header
	rateLimitN  int
	rateLimitAt time.Time
	malformed   map[string]bool
	calls       []string

	// handleCollaborators dispatches /collaborators?affiliation=… per
	// affiliation. The connector hits the same URL with different
	// affiliation values and the fake needs to return distinct bodies.
	handleCollaborators func(affiliation string) []byte
}

func newFakeProxy() *fakeProxy {
	return &fakeProxy{
		pages:     map[string][]byte{},
		defaults:  map[string][]byte{},
		headers:   map[string]http.Header{},
		malformed: map[string]bool{},
	}
}

func (f *fakeProxy) addPage(method, path string, pageNumber int, body []byte, nextPage int) {
	key := pageKey(method, path, pageNumber)
	f.pages[key] = body
	hdr := http.Header{}
	if nextPage > 0 {
		hdr.Set("Link", `<https://api.github.com`+path+`?page=`+strconv.Itoa(nextPage)+`>; rel="next"`)
	}
	f.headers[key] = hdr
}

func (f *fakeProxy) addDefault(method, path string, body []byte) {
	f.defaults[method+" "+path] = body
}

func (f *fakeProxy) injectRateLimit(n int) {
	f.rateLimitN = n
	f.rateLimitAt = time.Now().Add(50 * time.Millisecond)
}

func (f *fakeProxy) injectMalformed(method, path string, pageNumber int) {
	f.malformed[pageKey(method, path, pageNumber)] = true
}

func (f *fakeProxy) Do(
	_ context.Context, method, path string, query url.Values,
) (int, http.Header, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, method+" "+path+"?"+query.Encode())

	if f.rateLimitN > 0 {
		f.rateLimitN--
		hdr := http.Header{}
		hdr.Set("X-RateLimit-Remaining", "0")
		hdr.Set("X-RateLimit-Reset", strconv.FormatInt(f.rateLimitAt.Unix(), 10))
		return http.StatusForbidden, hdr, []byte(`{"message":"rate limit"}`), nil
	}

	page, _ := strconv.Atoi(query.Get("page"))
	if page == 0 {
		page = 1
	}
	if f.handleCollaborators != nil && strings.HasSuffix(path, "/collaborators") {
		body := f.handleCollaborators(query.Get("affiliation"))
		return http.StatusOK, http.Header{}, body, nil
	}
	key := pageKey(method, path, page)
	if f.malformed[key] {
		delete(f.malformed, key)
		return http.StatusOK, http.Header{}, []byte("{not valid json"), nil
	}
	if body, ok := f.pages[key]; ok {
		return http.StatusOK, cloneHeader(f.headers[key]), body, nil
	}
	if body, ok := f.defaults[method+" "+path]; ok {
		return http.StatusOK, http.Header{}, body, nil
	}
	return http.StatusNotFound, http.Header{}, []byte(`{"message":"not found"}`),
		errors.New("fakeProxy: no fixture for " + key)
}

func pageKey(method, path string, page int) string {
	return method + " " + path + "?page=" + strconv.Itoa(page)
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	clone := http.Header{}
	for k, v := range h {
		clone[k] = append([]string(nil), v...)
	}
	return clone
}

type fixtureSource struct {
	cfg json.RawMessage
}

func (s *fixtureSource) SourceID() string        { return "src-fixture" }
func (s *fixtureSource) OrgID() string           { return "org-fixture" }
func (s *fixtureSource) SourceKind() string      { return Kind }
func (s *fixtureSource) Config() json.RawMessage { return s.cfg }

func makePR(num int, state string, updated time.Time) GithubPR {
	return GithubPR{
		ID:        int64(num) * 100,
		Number:    num,
		Title:     fmt.Sprintf("PR %d", num),
		Body:      fmt.Sprintf("body of PR %d", num),
		State:     state,
		HTMLURL:   fmt.Sprintf("https://github.com/acme/widget/pull/%d", num),
		CreatedAt: updated.Add(-time.Hour),
		UpdatedAt: updated,
		User:      &GithubUser{Login: "alice", Email: "alice@example.com"},
	}
}
func makeIssue(num int, isPR bool, updated time.Time) GithubIssue {
	issue := GithubIssue{
		ID:        int64(num) * 200,
		Number:    num,
		Title:     fmt.Sprintf("Issue %d", num),
		Body:      fmt.Sprintf("body of issue %d", num),
		State:     "open",
		HTMLURL:   fmt.Sprintf("https://github.com/acme/widget/issues/%d", num),
		CreatedAt: updated,
		UpdatedAt: updated,
		User:      &GithubUser{Login: "bob", Email: "bob@example.com"},
	}
	if isPR {
		issue.PullRequest = &GithubPRRef{URL: "https://github.com/acme/widget/pull/" + strconv.Itoa(num)}
	}
	return issue
}
func makeRepo(visibility string) GithubRepo {
	return GithubRepo{
		ID:         100,
		Name:       "widget",
		FullName:   "acme/widget",
		Owner:      GithubRepoOwner{ID: 1, Login: "acme", Type: "Organization"},
		Private:    visibility != "public",
		Visibility: visibility,
		HTMLURL:    "https://github.com/acme/widget",
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

func drainIngest(t *testing.T, ch <-chan interfaces.DocumentOrFailure,
) (docs []*interfaces.Document, fails []*interfaces.ConnectorFailure) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Doc != nil {
				docs = append(docs, ev.Doc)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainIngest: timeout waiting for connector output")
		}
	}
}

func drainSlim(t *testing.T, ch <-chan interfaces.SlimDocOrFailure,
) (slims []*interfaces.SlimDocument, fails []*interfaces.ConnectorFailure) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Slim != nil {
				slims = append(slims, ev.Slim)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainSlim: timeout waiting for slim output")
		}
	}
}

func drainGroups(t *testing.T, ch <-chan interfaces.ExternalGroupOrFailure,
) (groups []*interfaces.ExternalGroup, fails []*interfaces.ConnectorFailure) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Group != nil {
				groups = append(groups, ev.Group)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainGroups: timeout waiting for group output")
		}
	}
}

func hasSubstring(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

const repoFullName = "acme/widget"
