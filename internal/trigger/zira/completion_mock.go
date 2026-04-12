package zira

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// MockCompletionClient is a test double for CompletionClient. It returns
// scripted response sequences keyed by the content of the first user
// message in the request. Each call to ChatCompletion pops the next
// response from the sequence for that key.
//
// Usage:
//
//	mock := NewMockCompletionClient()
//	mock.OnMessage("review this PR", resp1, resp2)  // first call returns resp1, second returns resp2
//	// inject mock into RouterAgent, run test
//	mock.AssertCallCount(t, 2)
type MockCompletionClient struct {
	mu        sync.Mutex
	calls     []CompletionRequest
	sequences map[string][]CompletionResponse
	indices   map[string]int
	fallback  *CompletionResponse
}

// NewMockCompletionClient creates a mock with no scripted responses.
func NewMockCompletionClient() *MockCompletionClient {
	return &MockCompletionClient{
		sequences: make(map[string][]CompletionResponse),
		indices:   make(map[string]int),
	}
}

// OnMessage registers a sequence of responses for requests whose first
// user message matches the given text. Successive ChatCompletion calls
// with the same user message return the next response in the sequence.
func (m *MockCompletionClient) OnMessage(userMessage string, responses ...CompletionResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sequences[userMessage] = responses
	m.indices[userMessage] = 0
}

// SetFallback sets a default response returned when no scripted sequence
// matches the user message.
func (m *MockCompletionClient) SetFallback(response CompletionResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallback = &response
}

// ChatCompletion implements CompletionClient. It records the request and
// returns the next scripted response for the matching user message.
func (m *MockCompletionClient) ChatCompletion(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)

	// Find the first user message to use as the lookup key.
	key := ""
	for _, message := range req.Messages {
		if message.Role == "user" {
			key = message.Content
			break
		}
	}

	if seq, ok := m.sequences[key]; ok {
		index := m.indices[key]
		if index < len(seq) {
			m.indices[key] = index + 1
			resp := seq[index]
			return &resp, nil
		}
		// Sequence exhausted — return last response again (stable state).
		return &seq[len(seq)-1], nil
	}

	if m.fallback != nil {
		resp := *m.fallback
		return &resp, nil
	}

	return nil, fmt.Errorf("mock: no scripted response for user message %q and no fallback set", key)
}

// Calls returns all recorded requests.
func (m *MockCompletionClient) Calls() []CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]CompletionRequest, len(m.calls))
	copy(result, m.calls)
	return result
}

// AssertCallCount asserts the mock was called exactly n times.
func (m *MockCompletionClient) AssertCallCount(t *testing.T, expected int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) != expected {
		t.Errorf("MockCompletionClient: expected %d calls, got %d", expected, len(m.calls))
	}
}

// LastRequest returns the most recent request, or panics if none.
func (m *MockCompletionClient) LastRequest() CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		panic("MockCompletionClient: no calls recorded")
	}
	return m.calls[len(m.calls)-1]
}

// Reset clears all recorded calls and resets sequence indices.
func (m *MockCompletionClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	for key := range m.indices {
		m.indices[key] = 0
	}
}
