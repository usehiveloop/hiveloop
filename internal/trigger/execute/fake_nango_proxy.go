package execute

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FakeNangoProxy is the test double for NangoProxy. It stores scripted
// responses keyed by "METHOD PATH" and records every call it receives so
// tests can assert on what the executor actually sent.
//
// Scripts can be set up in two ways:
//
//  1. Stub(method, path, response) — inline response map
//  2. StubFromFile(method, path, fixtureFile) — loads JSON from testdata/
//
// The fake matches purely on method + path (query strings and body are
// ignored for matching but are recorded on Calls). This matches the common
// test pattern of "given this API call, return this response" without
// forcing tests to duplicate query/body construction.
//
// FakeNangoProxy is safe for concurrent use — the executor doesn't parallelize
// context actions today, but the fake's mutex protects against test code that
// runs multiple executions in parallel (which happens in table-driven tests).
type FakeNangoProxy struct {
	mu           sync.Mutex
	stubs        map[string]any   // "METHOD PATH" -> response value (object, array, or scalar)
	errorStubs   map[string]error // "METHOD PATH" -> forced error
	fixturesRoot string           // base dir for StubFromFile relative paths

	// Calls is the ordered log of every proxy request the fake received,
	// in the order they arrived. Tests assert on this to verify the
	// executor fired the right actions with the right params.
	Calls []ProxyRequest
}

// NewFakeNangoProxy constructs an empty fake. fixturesRoot is the directory
// (relative to the test file or absolute) where StubFromFile will look for
// response JSON files. Typical value: "testdata/github/responses" or
// "testdata/slack/responses".
func NewFakeNangoProxy(fixturesRoot string) *FakeNangoProxy {
	return &FakeNangoProxy{
		stubs:        make(map[string]any),
		errorStubs:   make(map[string]error),
		fixturesRoot: fixturesRoot,
	}
}

// Stub registers a scripted response for a specific (method, path) pair.
// Calls to Proxy matching this pair will return the response value. The
// response can be any JSON-decoded shape: map[string]any, []any, string,
// number, bool. Use StubFromFile to load from a JSON file instead.
func (f *FakeNangoProxy) Stub(method, path string, response any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stubs[stubKey(method, path)] = response
}

// StubFromFile reads a JSON response fixture from the fixtures directory
// and registers it as the response for (method, path). The file path is
// relative to the fixturesRoot passed to NewFakeNangoProxy.
//
// The JSON can be any shape — object, array, scalar — mirroring what real
// provider responses look like. The fixture loader parses into `any` so
// arrays and objects round-trip naturally.
func (f *FakeNangoProxy) StubFromFile(method, path, fixtureFile string) error {
	data, err := os.ReadFile(filepath.Join(f.fixturesRoot, fixtureFile))
	if err != nil {
		return fmt.Errorf("stub fixture %q: %w", fixtureFile, err)
	}
	var body any
	if err := json.Unmarshal(data, &body); err != nil {
		return fmt.Errorf("parse fixture %q: %w", fixtureFile, err)
	}
	f.Stub(method, path, body)
	return nil
}

// StubError forces a specific (method, path) call to return an error
// instead of a response. Used for testing optional-context failure paths
// and error propagation.
func (f *FakeNangoProxy) StubError(method, path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errorStubs[stubKey(method, path)] = err
}

// Proxy is the NangoProxy interface method. It records the call, looks up
// a matching stub, and either returns the scripted response or a
// helpful "no stub registered" error listing all the keys the fake knows
// about (so test failures name the missing stub explicitly).
func (f *FakeNangoProxy) Proxy(ctx context.Context, req ProxyRequest) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, req)
	key := stubKey(req.Method, req.Path)

	if err, ok := f.errorStubs[key]; ok {
		return nil, err
	}
	if response, ok := f.stubs[key]; ok {
		return response, nil
	}

	available := make([]string, 0, len(f.stubs))
	for k := range f.stubs {
		available = append(available, k)
	}
	return nil, fmt.Errorf("fake nango: no stub registered for %q; available: %v", key, available)
}

// CallCount returns how many times Proxy was called. Tests use this to
// verify the executor didn't over- or under-fire context actions.
func (f *FakeNangoProxy) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

// CallFor returns all proxy requests that hit (method, path). Useful for
// asserting the body or query params of a specific call.
func (f *FakeNangoProxy) CallFor(method, path string) []ProxyRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ProxyRequest
	key := stubKey(method, path)
	for _, call := range f.Calls {
		if stubKey(call.Method, call.Path) == key {
			out = append(out, call)
		}
	}
	return out
}

func stubKey(method, path string) string {
	return method + " " + path
}
