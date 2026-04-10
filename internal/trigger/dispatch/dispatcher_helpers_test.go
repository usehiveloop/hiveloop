package dispatch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
)

// loadFixture reads a webhook payload JSON file from testdata/github/.
// Test names match fixture filenames so failures are easy to trace back.
func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	return loadFixtureFrom(t, "github", name)
}

// loadSlackFixture reads a webhook payload JSON file from testdata/slack/.
// Mirror of loadFixture for Slack-specific tests.
func loadSlackFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	return loadFixtureFrom(t, "slack", name)
}

// loadFixtureFrom is the shared implementation for provider-specific fixture
// loaders. The provider argument names the subdirectory under testdata/.
func loadFixtureFrom(t *testing.T, provider, name string) map[string]any {
	t.Helper()
	path := filepath.Join("testdata", provider, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadFixtureFrom(%s, %s): %v", provider, name, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("loadFixtureFrom(%s, %s): json decode: %v", provider, name, err)
	}
	return payload
}

// dispatchHarness bundles the dispatcher under test with its in-memory stores
// and shared identifiers so tests stay terse. Each test constructs a fresh
// harness — they're cheap.
type dispatchHarness struct {
	t            *testing.T
	dispatcher   *Dispatcher
	store        *MemoryAgentTriggerStore
	orgID        uuid.UUID
	connectionID uuid.UUID
	connection   *model.Connection
}

// newHarness builds a dispatcher backed by in-memory stores and the real
// embedded catalog (so trigger refs/resource bindings come from production
// data — no mocking that surface).
func newHarness(t *testing.T) *dispatchHarness {
	t.Helper()
	store := NewMemoryAgentTriggerStore()

	// Discard logger for tests; flip to slog.NewTextHandler(os.Stderr, nil) when
	// debugging a specific case.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	connectionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	integrationID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	connection := &model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		IntegrationID:     integrationID,
		NangoConnectionID: "nango-conn-test",
		Integration: model.Integration{
			ID:        integrationID,
			OrgID:     orgID,
			Provider:  "github-app",
			UniqueKey: "github_app",
		},
	}

	return &dispatchHarness{
		t:            t,
		dispatcher:   New(store, catalog.Global(), logger),
		store:        store,
		orgID:        orgID,
		connectionID: connectionID,
		connection:   connection,
	}
}

// addTrigger seeds an agent + agent trigger pair. The default agent is shared
// (sandbox pool); pass a customizer to override SandboxType, Integrations, etc.
func (h *dispatchHarness) addTrigger(triggerKeys []string, conditions *model.TriggerMatch, contextActions []model.ContextAction, instructions string, customizers ...func(*model.Agent, *model.AgentTrigger)) (model.Agent, model.AgentTrigger) {
	h.t.Helper()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := model.Agent{
		ID:           agentID,
		OrgID:        &h.orgID,
		Name:         "test-agent",
		SandboxType:  "shared",
		SandboxID:    &sandboxID,
		SystemPrompt: "you are a test agent",
		Model:        "claude-opus-4-6",
	}

	conditionsJSON := mustMarshalJSON(h.t, conditions)
	contextActionsJSON := mustMarshalJSON(h.t, contextActions)

	trigger := model.AgentTrigger{
		ID:             uuid.New(),
		OrgID:          h.orgID,
		AgentID:        agentID,
		ConnectionID:   h.connectionID,
		TriggerKeys:    triggerKeys,
		Enabled:        true,
		Conditions:     conditionsJSON,
		ContextActions: contextActionsJSON,
		Instructions:   instructions,
	}

	for _, customize := range customizers {
		customize(&agent, &trigger)
	}

	h.store.Add(agent, trigger)
	return agent, trigger
}

// addTerminateTrigger seeds an agent + agent trigger pair that has terminate
// rules. The denormalized TerminateEventKeys column is populated from the
// rules exactly the way the production handler does it, so store matching
// works identically to real webhooks.
func (h *dispatchHarness) addTerminateTrigger(
	triggerKeys []string,
	conditions *model.TriggerMatch,
	contextActions []model.ContextAction,
	instructions string,
	terminateRules []model.TerminateRule,
	customizers ...func(*model.Agent, *model.AgentTrigger),
) (model.Agent, model.AgentTrigger) {
	h.t.Helper()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agent := model.Agent{
		ID:           agentID,
		OrgID:        &h.orgID,
		Name:         "test-agent",
		SandboxType:  "shared",
		SandboxID:    &sandboxID,
		SystemPrompt: "you are a test agent",
		Model:        "claude-opus-4-6",
	}

	trigger := model.AgentTrigger{
		ID:                 uuid.New(),
		OrgID:              h.orgID,
		AgentID:            agentID,
		ConnectionID:       h.connectionID,
		TriggerKeys:        triggerKeys,
		Enabled:            true,
		Conditions:         mustMarshalJSON(h.t, conditions),
		ContextActions:     mustMarshalJSON(h.t, contextActions),
		Instructions:       instructions,
		TerminateOn:        mustMarshalJSON(h.t, terminateRules),
		TerminateEventKeys: model.CollectTerminateEventKeys(terminateRules),
	}

	for _, customize := range customizers {
		customize(&agent, &trigger)
	}

	h.store.Add(agent, trigger)
	return agent, trigger
}

func mustMarshalJSON(t *testing.T, value any) model.RawJSON {
	t.Helper()
	if value == nil {
		return nil
	}
	// Don't store an empty object/array as JSON null.
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(bytes) == "null" {
		return nil
	}
	return model.RawJSON(bytes)
}

// run executes a dispatch with the harness's connection and returns the runs.
// Test functions call this directly with the event metadata + payload. The
// provider is pulled from the harness's connection so GitHub and Slack
// harnesses use the same helper without branching.
func (h *dispatchHarness) run(eventType, eventAction string, payload map[string]any) []PreparedRun {
	h.t.Helper()
	input := DispatchInput{
		Provider:    h.connection.Integration.Provider,
		EventType:   eventType,
		EventAction: eventAction,
		Payload:     payload,
		DeliveryID:  "test-delivery",
		OrgID:       h.orgID,
		Connection:  h.connection,
	}
	runs, err := h.dispatcher.Run(context.Background(), input)
	if err != nil {
		h.t.Fatalf("dispatcher.Run: %v", err)
	}
	return runs
}

// newSlackHarness builds a dispatcher harness wired for the Slack provider.
// Same pattern as newHarness but the connection's Integration provider is
// "slack" instead of "github-app". Uses the real embedded Slack catalog —
// trigger refs, resource templates, and coalescing behavior all come from
// production data in slack.triggers.json and slack.actions.json.
func newSlackHarness(t *testing.T) *dispatchHarness {
	t.Helper()
	store := NewMemoryAgentTriggerStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	connectionID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	integrationID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

	connection := &model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		IntegrationID:     integrationID,
		NangoConnectionID: "nango-slack-test",
		Integration: model.Integration{
			ID:        integrationID,
			OrgID:     orgID,
			Provider:  "slack",
			UniqueKey: "slack_workspace",
		},
	}

	return &dispatchHarness{
		t:            t,
		dispatcher:   New(store, catalog.Global(), logger),
		store:        store,
		orgID:        orgID,
		connectionID: connectionID,
		connection:   connection,
	}
}

// assertSinglePrepared asserts exactly one PreparedRun was returned and panics
// the test on mismatch with the count, skip reasons, and a one-line summary.
func assertSinglePrepared(t *testing.T, runs []PreparedRun) PreparedRun {
	t.Helper()
	if len(runs) != 1 {
		t.Fatalf("expected 1 prepared run, got %d", len(runs))
	}
	return runs[0]
}

// assertContextRequest finds a context request by its `as` name. Failing this
// assert is the most common diagnosis path so we want a descriptive failure.
func assertContextRequest(t *testing.T, run PreparedRun, as string) ContextRequest {
	t.Helper()
	for _, request := range run.ContextRequests {
		if request.As == as {
			return request
		}
	}
	available := make([]string, 0, len(run.ContextRequests))
	for _, request := range run.ContextRequests {
		available = append(available, request.As)
	}
	t.Fatalf("context request %q not found; available: %v", as, available)
	return ContextRequest{}
}
