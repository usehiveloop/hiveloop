package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

// --------------------------------------------------------------------------
// buildCronDispatchInstructions tests
// --------------------------------------------------------------------------

func TestBuildCronDispatchInstructions_WithPersonaAndInstructions(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		RouterPersona:       "You are a helpful teammate.",
		TriggerInstructions: "Run your daily standup check.",
	}
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	result := buildCronDispatchInstructions(agentDispatch, scheduledAt)

	if result == "" {
		t.Fatal("expected non-empty instructions")
	}

	// Should contain persona.
	if !containsString(result, "You are a helpful teammate.") {
		t.Error("missing persona in instructions")
	}
	// Should contain trigger instructions.
	if !containsString(result, "Run your daily standup check.") {
		t.Error("missing trigger instructions")
	}
	// Should contain scheduled time.
	if !containsString(result, "2026-04-19T09:00:00Z") {
		t.Error("missing scheduled time in instructions")
	}
}

func TestBuildCronDispatchInstructions_NoPersona(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		TriggerInstructions: "Check for open PRs.",
	}
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	result := buildCronDispatchInstructions(agentDispatch, scheduledAt)

	if containsString(result, "---") {
		t.Error("should not contain persona separator when persona is empty")
	}
	if !containsString(result, "Check for open PRs.") {
		t.Error("missing trigger instructions")
	}
	if !containsString(result, "Scheduled run at:") {
		t.Error("missing scheduled time")
	}
}

func TestBuildCronDispatchInstructions_NoInstructions(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		RouterPersona: "You are Zira.",
	}
	scheduledAt := time.Date(2026, 4, 19, 14, 30, 0, 0, time.UTC)

	result := buildCronDispatchInstructions(agentDispatch, scheduledAt)

	if !containsString(result, "You are Zira.") {
		t.Error("missing persona")
	}
	if !containsString(result, "2026-04-19T14:30:00Z") {
		t.Error("missing scheduled time")
	}
}

func TestBuildCronDispatchInstructions_SubstitutesRefs(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		TriggerInstructions: "Check $refs.repo for open PRs in $refs.environment.",
		Refs:                map[string]string{"repo": "ziraloop", "environment": "production"},
	}
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	result := buildCronDispatchInstructions(agentDispatch, scheduledAt)

	if containsString(result, "$refs.repo") {
		t.Error("$refs.repo should have been substituted")
	}
	if containsString(result, "$refs.environment") {
		t.Error("$refs.environment should have been substituted")
	}
	if !containsString(result, "Check ziraloop for open PRs in production.") {
		t.Errorf("expected substituted instructions, got: %s", result)
	}
	if !containsString(result, "Scheduled run at:") {
		t.Error("missing scheduled time")
	}
}

func TestBuildCronDispatchInstructions_MustacheRefs(t *testing.T) {
	agentDispatch := dispatch.AgentDispatch{
		TriggerInstructions: "Deploy {{$refs.service}} to {{$refs.target}}.",
		Refs:                map[string]string{"service": "api", "target": "staging"},
	}
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	result := buildCronDispatchInstructions(agentDispatch, scheduledAt)

	if !containsString(result, "Deploy api to staging.") {
		t.Errorf("expected mustache refs substituted, got: %s", result)
	}
}

// --------------------------------------------------------------------------
// CronTriggerDispatchPayload serialization
// --------------------------------------------------------------------------

func TestCronTriggerDispatchPayload_RoundTrip(t *testing.T) {
	triggerID := uuid.New()
	orgID := uuid.New()
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)

	original := CronTriggerDispatchPayload{
		RouterTriggerID: triggerID,
		OrgID:           orgID,
		ScheduledAt:     scheduledAt,
	}

	task, err := NewCronTriggerDispatchTask(original)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if task.Type() != TypeCronTriggerDispatch {
		t.Errorf("task type: got %q, want %q", task.Type(), TypeCronTriggerDispatch)
	}

	var decoded CronTriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if decoded.RouterTriggerID != triggerID {
		t.Errorf("trigger ID: got %s, want %s", decoded.RouterTriggerID, triggerID)
	}
	if decoded.OrgID != orgID {
		t.Errorf("org ID: got %s, want %s", decoded.OrgID, orgID)
	}
	if !decoded.ScheduledAt.Equal(scheduledAt) {
		t.Errorf("scheduled at: got %v, want %v", decoded.ScheduledAt, scheduledAt)
	}
}

// --------------------------------------------------------------------------
// TriggerDispatchPayload with RouterTriggerID
// --------------------------------------------------------------------------

func TestTriggerDispatchPayload_RouterTriggerID_RoundTrip(t *testing.T) {
	triggerID := uuid.New()

	original := TriggerDispatchPayload{
		Provider:        "http",
		EventType:       "http",
		DeliveryID:      "test-123",
		OrgID:           uuid.New(),
		PayloadJSON:     []byte(`{"action":"deploy"}`),
		RouterTriggerID: &triggerID,
	}

	task, err := NewRouterDispatchTask(original)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	var decoded TriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.RouterTriggerID == nil {
		t.Fatal("RouterTriggerID should not be nil")
	}
	if *decoded.RouterTriggerID != triggerID {
		t.Errorf("trigger ID: got %s, want %s", *decoded.RouterTriggerID, triggerID)
	}
}

func TestTriggerDispatchPayload_RouterTriggerID_OmittedWhenNil(t *testing.T) {
	original := TriggerDispatchPayload{
		Provider:    "github",
		EventType:   "pull_request",
		EventAction: "opened",
		DeliveryID:  "test-456",
		OrgID:       uuid.New(),
		PayloadJSON: []byte(`{}`),
	}

	task, err := NewRouterDispatchTask(original)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	var decoded TriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.RouterTriggerID != nil {
		t.Errorf("RouterTriggerID should be nil when not set, got %s", *decoded.RouterTriggerID)
	}
}

// --------------------------------------------------------------------------
// CronTriggerDispatchHandler unit tests (with mock dispatcher)
// --------------------------------------------------------------------------

func TestCronTriggerDispatchHandler_EnqueuesConversationCreate(t *testing.T) {
	// Build a test dispatcher with an in-memory store.
	triggerID := uuid.New()
	orgID := uuid.MustParse("11111111-0000-0000-0000-000000000001")
	agentID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	routerID := uuid.MustParse("33333333-0000-0000-0000-000000000001")

	store := dispatch.NewMemoryRouterTriggerStore()
	trigger := dispatch.RouterTriggerForTest(triggerID, orgID, routerID, "cron", "Run daily check.")
	router := dispatch.RouterForTest(routerID, orgID, "You are Zira.", &agentID)
	store.AddTrigger(trigger, router)
	store.AddRule(triggerID, dispatch.RuleForTest(agentID, 1))

	agentOrgID := orgID
	store.AddAgent(dispatch.AgentForTest(agentID, &agentOrgID, "daily-agent"))

	mockEnqueuer := &enqueue.MockClient{}
	dispatcher := dispatch.NewRouterDispatcher(store, catalog.Global(), nil, nil)
	handler := NewCronTriggerDispatchHandler(dispatcher, mockEnqueuer)

	// Build the task payload.
	scheduledAt := time.Date(2026, 4, 19, 9, 0, 0, 0, time.UTC)
	payload := CronTriggerDispatchPayload{
		RouterTriggerID: triggerID,
		OrgID:           orgID,
		ScheduledAt:     scheduledAt,
	}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeCronTriggerDispatch, payloadBytes)

	// Execute the handler.
	err := handler.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Verify an AgentConversationCreate task was enqueued.
	mockEnqueuer.AssertEnqueued(t, TypeAgentConversationCreate)

	tasks := mockEnqueuer.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(tasks))
	}

	// Verify the payload contains the correct agent and instructions.
	var convPayload AgentConversationCreatePayload
	if err := json.Unmarshal(tasks[0].Payload, &convPayload); err != nil {
		t.Fatalf("failed to unmarshal conversation create payload: %v", err)
	}
	if convPayload.AgentID != agentID {
		t.Errorf("agent ID: got %s, want %s", convPayload.AgentID, agentID)
	}
	if convPayload.RouterTriggerID != triggerID {
		t.Errorf("trigger ID: got %s, want %s", convPayload.RouterTriggerID, triggerID)
	}
	if !containsString(convPayload.Instructions, "Run daily check.") {
		t.Errorf("instructions should contain trigger instructions, got: %s", convPayload.Instructions)
	}
	if !containsString(convPayload.Instructions, "2026-04-19T09:00:00Z") {
		t.Errorf("instructions should contain scheduled time, got: %s", convPayload.Instructions)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || findSubstring(haystack, needle))
}

func findSubstring(haystack, needle string) bool {
	for index := 0; index <= len(haystack)-len(needle); index++ {
		if haystack[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
