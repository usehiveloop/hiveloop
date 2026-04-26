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

// fakeProxy is the test-side implementation of proxyClient. It serves
// pre-recorded JSON keyed on (method, path, page) and supports two
// deliberately-injected failure modes used by the failure tests:
//
//   - injectRateLimit(n): the next n calls return 403 with the
//     X-RateLimit-* headers populated.
//   - injectMalformed(method, path, page): the next matching call
//     returns invalid JSON, exercising per-page failure paths.
type fakeProxy struct {
	mu          sync.Mutex
	pages       map[string][]byte
	defaults    map[string][]byte
	headers     map[string]http.Header
	rateLimitN  int
	rateLimitAt time.Time
	malformed   map[string]bool
	calls       []string

	// handleCollaborators, when set, is used to dispatch
	// /collaborators?affiliation=... on a per-affiliation basis. The
	// connector calls the same URL twice with different `affiliation`
	// values and the fake proxy needs to return different bodies. Tests
	// that don't care about this distinction leave it nil and use
	// addPage/addDefault.
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

// addPage registers a response for a paginated endpoint. nextPage > 0
// populates a Link header advertising rel="next".
func (f *fakeProxy) addPage(method, path string, pageNumber int, body []byte, nextPage int) {
	key := pageKey(method, path, pageNumber)
	f.pages[key] = body
	hdr := http.Header{}
	if nextPage > 0 {
		hdr.Set("Link", `<https://api.github.com`+path+`?page=`+strconv.Itoa(nextPage)+`>; rel="next"`)
	}
	f.headers[key] = hdr
}

// addDefault registers a non-paginated response.
func (f *fakeProxy) addDefault(method, path string, body []byte) {
	f.defaults[method+" "+path] = body
}

// injectRateLimit sets the next n calls to return 403 + RateLimit headers.
func (f *fakeProxy) injectRateLimit(n int) {
	f.rateLimitN = n
	f.rateLimitAt = time.Now().Add(50 * time.Millisecond)
}

// injectMalformed marks the next hit on (method, path, page) to return
// invalid JSON.
func (f *fakeProxy) injectMalformed(method, path string, pageNumber int) {
	f.malformed[pageKey(method, path, pageNumber)] = true
}

// Do implements proxyClient.
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

// fixtureSource implements interfaces.Source for a config map.
type fixtureSource struct {
	cfg json.RawMessage
}

func (s *fixtureSource) SourceID() string        { return "src-fixture" }
func (s *fixtureSource) OrgID() string           { return "org-fixture" }
func (s *fixtureSource) SourceKind() string      { return Kind }
func (s *fixtureSource) Config() json.RawMessage { return s.cfg }

// makePR / makeIssue / makeRepo build minimal fixture entities.
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

// mustMarshal is a fixture-builder convenience. JSON failures here are
// programming errors — tests fail fast.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return b
}

// drainIngest collects every DocumentOrFailure from a connector channel
// with a 5s timeout — long enough for any retry path, short enough that
// a hung test still finishes.
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

// drainSlim is the SlimDocOrFailure twin of drainIngest.
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

// drainGroups is the ExternalGroupOrFailure twin.
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

// hasSubstring is a small convenience used by failure-message asserts.
func hasSubstring(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// repoFullName is the canonical fixture under test.
const repoFullName = "acme/widget"
