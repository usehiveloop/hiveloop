package execute

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
	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
)

// The executor tests are end-to-end: they build a DispatchInput from a real
// webhook fixture, run the real dispatcher against the real embedded
// catalog, and then feed the resulting PreparedRun through the executor
// with a FakeNangoProxy scripted to return realistic response bodies.
//
// The only thing mocked is Nango. Everything else — catalog data, ref
// extraction, template substitution, conditions, context action resolution,
// instruction assembly — runs production code paths against production data.
//
// This mirrors how dispatcher tests work (real catalog, real fixtures, only
// the store is in-memory) so failures at either layer surface the same way.

// pipelineHarness bundles a dispatcher harness + an executor so tests can
// run a webhook → final instruction pipeline in one step.
type pipelineHarness struct {
	t            *testing.T
	dispatcher   *dispatch.Dispatcher
	store        *dispatch.MemoryAgentTriggerStore
	executor     *Executor
	fakeNango    *FakeNangoProxy
	orgID        uuid.UUID
	connectionID uuid.UUID
	connection   *model.Connection
}

// newGitHubPipelineHarness wires up the full dispatcher + executor pipeline
// for a github-app connection. Fixtures and response stubs come from
// testdata/github/.
func newGitHubPipelineHarness(t *testing.T) *pipelineHarness {
	t.Helper()
	return newPipelineHarness(t, "github-app", "github_app", "github")
}

// newSlackPipelineHarness wires up the same pipeline for a slack connection.
// Fixtures and responses come from testdata/slack/.
func newSlackPipelineHarness(t *testing.T) *pipelineHarness {
	t.Helper()
	return newPipelineHarness(t, "slack", "slack_workspace", "slack")
}

func newPipelineHarness(t *testing.T, provider, uniqueKey, fixturesProvider string) *pipelineHarness {
	t.Helper()

	store := dispatch.NewMemoryAgentTriggerStore()
	// Discard logger by default; test authors can rewire by setting
	// harness.executor.Logger + harness.dispatcher.Logger if they need
	// to debug a specific case.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cat := catalog.Global()
	dispatcher := dispatch.New(store, cat, logger)

	fakeNango := NewFakeNangoProxy(filepath.Join("testdata", fixturesProvider, "responses"))
	executor, err := New(fakeNango, cat, logger)
	if err != nil {
		t.Fatalf("New executor: %v", err)
	}

	// Stable IDs so tests can assert on them without cross-test flakiness.
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	connectionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	integrationID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	connection := &model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		IntegrationID:     integrationID,
		NangoConnectionID: "nango-conn-" + fixturesProvider,
		Integration: model.Integration{
			ID:        integrationID,
			OrgID:     orgID,
			Provider:  provider,
			UniqueKey: uniqueKey,
		},
	}

	return &pipelineHarness{
		t:            t,
		dispatcher:   dispatcher,
		store:        store,
		executor:     executor,
		fakeNango:    fakeNango,
		orgID:        orgID,
		connectionID: connectionID,
		connection:   connection,
	}
}

// addAgentTrigger seeds one agent + trigger pair with sensible defaults. The
// customizer callback lets tests mutate the trigger (e.g., switch sandbox
// type, add terminate rules) before insertion.
func (h *pipelineHarness) addAgentTrigger(
	triggerKeys []string,
	conditions *model.TriggerMatch,
	contextActions []model.ContextAction,
	instructions string,
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
		ID:             uuid.New(),
		OrgID:          h.orgID,
		AgentID:        agentID,
		ConnectionID:   h.connectionID,
		TriggerKeys:    triggerKeys,
		Enabled:        true,
		Conditions:     marshalJSON(h.t, conditions),
		ContextActions: marshalJSON(h.t, contextActions),
		Instructions:   instructions,
	}

	for _, customize := range customizers {
		customize(&agent, &trigger)
	}

	h.store.Add(agent, trigger)
	return agent, trigger
}

// runPipeline runs the full pipeline: dispatcher with a webhook, then
// executor on the resulting (first) PreparedRun. Fails the test if the
// dispatcher returns zero runs or more than one — tests that want fan-out
// should call dispatchOnly and iterate explicitly.
func (h *pipelineHarness) runPipeline(eventType, eventAction, fixtureFile string) *ExecutedRun {
	h.t.Helper()

	payload := h.loadFixture(fixtureFile)
	input := dispatch.DispatchInput{
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
	if len(runs) != 1 {
		h.t.Fatalf("expected 1 prepared run, got %d", len(runs))
	}

	result, err := h.executor.Execute(context.Background(), runs[0])
	if err != nil {
		h.t.Fatalf("executor.Execute: %v", err)
	}
	return result
}

// loadFixture reads a webhook payload from the provider's testdata directory.
// The path is relative to where the test runs — by convention, the webhook
// fixtures live in ../dispatch/testdata/<provider>/ because the dispatcher
// already has them there. We don't duplicate the fixtures in the execute
// package; we reach into the dispatcher's testdata.
func (h *pipelineHarness) loadFixture(name string) map[string]any {
	h.t.Helper()
	provider := "github"
	if h.connection.Integration.Provider == "slack" {
		provider = "slack"
	}
	path := filepath.Join("..", "dispatch", "testdata", provider, name)
	data, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("loadFixture(%s): %v", name, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		h.t.Fatalf("loadFixture(%s): json decode: %v", name, err)
	}
	return payload
}

// stubResponse loads a response fixture from the executor's own testdata
// directory and registers it as the scripted reply for (method, path).
func (h *pipelineHarness) stubResponse(method, path, fixtureFile string) {
	h.t.Helper()
	if err := h.fakeNango.StubFromFile(method, path, fixtureFile); err != nil {
		h.t.Fatalf("stubResponse(%s %s, %s): %v", method, path, fixtureFile, err)
	}
}

// stubError forces a specific (method, path) call to fail. Used for
// optional-context failure tests.
func (h *pipelineHarness) stubError(method, path string, err error) {
	h.t.Helper()
	h.fakeNango.StubError(method, path, err)
}

func marshalJSON(t *testing.T, value any) model.RawJSON {
	t.Helper()
	if value == nil {
		return nil
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(bytes) == "null" {
		return nil
	}
	return model.RawJSON(bytes)
}

// buildDispatchInput constructs a DispatchInput from the harness. Used by
// tests that need to call dispatcher.Run and executor.Execute separately
// (e.g., to capture both results or to test error paths).
func buildDispatchInput(h *pipelineHarness, eventType, eventAction string, payload map[string]any) dispatch.DispatchInput {
	return dispatch.DispatchInput{
		Provider:    h.connection.Integration.Provider,
		EventType:   eventType,
		EventAction: eventAction,
		Payload:     payload,
		DeliveryID:  "test-delivery",
		OrgID:       h.orgID,
		Connection:  h.connection,
	}
}

// ctxFor returns a context.Background for test use. Wrapped in a helper
// so tests that need deadlines or cancellation can swap it easily.
func ctxFor() context.Context {
	return context.Background()
}
