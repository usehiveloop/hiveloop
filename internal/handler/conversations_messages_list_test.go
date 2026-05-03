package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const messagesListTestDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable" // #nosec G101

type messagesListHarness struct {
	db      *gorm.DB
	router  *chi.Mux
	org     model.Org
	agent   model.Agent
	sandbox model.Sandbox
	conv    model.AgentConversation
}

func newMessagesListHarness(t *testing.T) *messagesListHarness {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = messagesListTestDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("msglist-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	orgID := org.ID
	agent := model.Agent{
		ID:           uuid.New(),
		OrgID:        &orgID,
		Name:         "test-agent",
		SystemPrompt: "you are a test",
		Model:        "anthropic/claude-test",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	sb := model.Sandbox{
		ID:                    uuid.New(),
		OrgID:                 &orgID,
		AgentID:               &agent.ID,
		ExternalID:            "test-sb",
		BridgeURL:             "http://localhost:25434",
		EncryptedBridgeAPIKey: []byte{0x00},
		Status:                "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	conv := model.AgentConversation{
		ID:                   uuid.New(),
		OrgID:                org.ID,
		AgentID:              agent.ID,
		SandboxID:            sb.ID,
		BridgeConversationID: "ses_test",
		Status:               "active",
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conv: %v", err)
	}

	t.Cleanup(func() {
		db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	h := handler.NewConversationHandler(db, nil, nil, nil)
	r := chi.NewRouter()
	r.Route("/v1/conversations/{convID}", func(r chi.Router) {
		r.Get("/messages", h.ListMessages)
	})

	return &messagesListHarness{db: db, router: r, org: org, agent: agent, sandbox: sb, conv: conv}
}

func (h *messagesListHarness) seed(t *testing.T, events []model.ConversationEvent) {
	t.Helper()
	for i := range events {
		events[i].OrgID = h.org.ID
		events[i].ConversationID = h.conv.ID
		events[i].AgentID = h.agent.ID.String()
		events[i].BridgeConversationID = h.conv.BridgeConversationID
		if events[i].EventID == "" {
			events[i].EventID = uuid.New().String()
		}
		if events[i].Timestamp.IsZero() {
			events[i].Timestamp = time.Now()
		}
		if len(events[i].Data) == 0 {
			events[i].Data = model.RawJSON("{}")
		}
		if err := h.db.Create(&events[i]).Error; err != nil {
			t.Fatalf("insert event seq=%d: %v", events[i].SequenceNumber, err)
		}
	}
}

func (h *messagesListHarness) get(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := fmt.Sprintf("/v1/conversations/%s/messages", h.conv.ID)
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = middleware.WithOrg(req, &h.org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func mustJSON(t *testing.T, m map[string]any) model.RawJSON {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return model.RawJSON(b)
}

func decodeMessagesPage(t *testing.T, rr *httptest.ResponseRecorder) struct {
	Data        []map[string]any `json:"data"`
	LatestTodos []map[string]any `json:"latest_todos"`
	NextCursor  *string          `json:"next_cursor"`
	HasMore     bool             `json:"has_more"`
} {
	t.Helper()
	var resp struct {
		Data        []map[string]any `json:"data"`
		LatestTodos []map[string]any `json:"latest_todos"`
		NextCursor  *string          `json:"next_cursor"`
		HasMore     bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	return resp
}

func TestListMessages_AggregatesUserMessageAndConsecutiveBashCalls(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "message_received", Data: mustJSON(t, map[string]any{"content": "do the thing"})},
		// Three bash completions in a row with the same title should fold into one group.
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-1",
			"raw_output": map[string]any{"output": "a\nb\nc"},
		})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-2",
			"raw_output": map[string]any{"output": "hi"},
		})},
		{SequenceNumber: 4, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title": "bash", "tool_call_id": "tc-3",
			"raw_output": map[string]any{"output": "/work"},
		})},
		{SequenceNumber: 5, EventType: "turn_completed", Data: mustJSON(t, map[string]any{"stop_reason": "endturn"})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	page := decodeMessagesPage(t, rr)

	if got, want := len(page.Data), 2; got != want {
		t.Fatalf("messages: got %d, want %d", got, want)
	}

	user := page.Data[0]
	if user["author"] != "user" {
		t.Errorf("first message author: got %v, want user", user["author"])
	}
	if user["body"] != "do the thing" {
		t.Errorf("user body: got %v", user["body"])
	}

	agent := page.Data[1]
	if agent["author"] != "agent" {
		t.Errorf("second message author: got %v, want agent", agent["author"])
	}
	groups, _ := agent["tool_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("tool_groups: got %d, want 1 (consecutive bash calls should collapse)", len(groups))
	}
	g0 := groups[0].(map[string]any)
	if g0["name"] != "bash" {
		t.Errorf("group name: got %v", g0["name"])
	}
	calls, _ := g0["calls"].([]any)
	if len(calls) != 3 {
		t.Fatalf("calls in bash group: got %d, want 3", len(calls))
	}
	for i, c := range calls {
		cm := c.(map[string]any)
		if cm["status"] != "completed" {
			t.Errorf("call %d status: got %v, want completed", i, cm["status"])
		}
	}
	if calls[0].(map[string]any)["title"] != "bash" {
		t.Errorf("call 0 title: got %v, want bash", calls[0].(map[string]any)["title"])
	}
}

func TestListMessages_DifferentToolNamesProduceSeparateGroups(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "read", "tool_call_id": "b"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "c"})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1", len(page.Data))
	}
	groups := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 3 {
		t.Fatalf("groups: got %d, want 3 (bash → read → bash)", len(groups))
	}
	wantNames := []string{"bash", "read", "bash"}
	for i, g := range groups {
		if g.(map[string]any)["name"] != wantNames[i] {
			t.Errorf("group %d name: got %v, want %s", i, g.(map[string]any)["name"], wantNames[i])
		}
	}
}

func TestListMessages_TurnCompletedSeparatesAgentMessages(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "turn_completed", Data: mustJSON(t, map[string]any{"stop_reason": "endturn"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "b"})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 agent messages split by turn_completed, got %d", len(page.Data))
	}
	for i, m := range page.Data {
		if m["author"] != "agent" {
			t.Errorf("message %d author: got %v", i, m["author"])
		}
	}
}

func TestListMessages_StartedWithoutCompletionIsHidden(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 0 {
		t.Fatalf("messages: got %d, want 0 (a started-only call should produce no message)", len(page.Data))
	}
}

func TestListMessages_PaginationByCursor(t *testing.T) {
	h := newMessagesListHarness(t)

	events := []model.ConversationEvent{}
	for i := int64(1); i <= 6; i++ {
		events = append(events, model.ConversationEvent{
			SequenceNumber: i,
			EventType:      "message_received",
			Data:           mustJSON(t, map[string]any{"content": fmt.Sprintf("msg %d", i)}),
		})
	}
	h.seed(t, events)

	rr := h.get(t, "limit=3")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	page := decodeMessagesPage(t, rr)
	if !page.HasMore {
		t.Errorf("expected has_more=true on first page")
	}
	if page.NextCursor == nil {
		t.Fatalf("expected next_cursor on first page")
	}
	if len(page.Data) != 3 {
		t.Fatalf("first page len: got %d, want 3", len(page.Data))
	}
	if page.Data[0]["body"] != "msg 1" || page.Data[2]["body"] != "msg 3" {
		t.Errorf("first page bodies: got %v, %v", page.Data[0]["body"], page.Data[2]["body"])
	}

	rr2 := h.get(t, "limit=3&cursor="+*page.NextCursor)
	page2 := decodeMessagesPage(t, rr2)
	if page2.HasMore {
		t.Errorf("expected has_more=false on last page")
	}
	if len(page2.Data) != 3 {
		t.Fatalf("second page len: got %d, want 3", len(page2.Data))
	}
	if page2.Data[0]["body"] != "msg 4" || page2.Data[2]["body"] != "msg 6" {
		t.Errorf("second page bodies: got %v, %v", page2.Data[0]["body"], page2.Data[2]["body"])
	}
}

func TestListMessages_StartedEventsAreIgnored(t *testing.T) {
	h := newMessagesListHarness(t)

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "a"})},
		{SequenceNumber: 2, EventType: "tool_call_started", Data: mustJSON(t, map[string]any{"title": "bash", "tool_call_id": "b"})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "ls",
			"tool_call_id": "a",
			"raw_output":   map[string]any{"output": "ok"},
		})},
	})

	rr := h.get(t, "")
	page := decodeMessagesPage(t, rr)
	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1 (one completion → one agent message)", len(page.Data))
	}
	groups := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups: got %d, want 1", len(groups))
	}
	calls := groups[0].(map[string]any)["calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1 (only the completed event surfaces; the two started events are dropped)", len(calls))
	}
	if calls[0].(map[string]any)["status"] != "completed" {
		t.Errorf("call status: got %v, want completed", calls[0].(map[string]any)["status"])
	}
}

func TestListMessages_LatestTodosExtractedAndCallSuppressed(t *testing.T) {
	h := newMessagesListHarness(t)

	earlier := []any{
		map[string]any{"content": "old item", "status": "completed", "priority": "high"},
	}
	later := []any{
		map[string]any{"content": "do thing 1", "status": "completed", "priority": "high"},
		map[string]any{"content": "do thing 2", "status": "in_progress", "priority": "medium"},
		map[string]any{"content": "do thing 3", "status": "pending", "priority": "low"},
	}

	h.seed(t, []model.ConversationEvent{
		{SequenceNumber: 1, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "1 todos",
			"tool_call_id": "tw-1",
			"raw_output":   map[string]any{"metadata": map[string]any{"todos": earlier}},
		})},
		{SequenceNumber: 2, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "bash",
			"tool_call_id": "b-1",
			"raw_output":   map[string]any{"output": "ok"},
		})},
		{SequenceNumber: 3, EventType: "tool_call_completed", Data: mustJSON(t, map[string]any{
			"title":        "3 todos",
			"tool_call_id": "tw-2",
			"raw_output":   map[string]any{"metadata": map[string]any{"todos": later}},
		})},
	})

	rr := h.get(t, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	page := decodeMessagesPage(t, rr)

	// Only the bash tool call should surface in messages — both todo writes
	// are suppressed in favor of the dedicated latest_todos field.
	if len(page.Data) != 1 {
		t.Fatalf("messages: got %d, want 1", len(page.Data))
	}
	groups, _ := page.Data[0]["tool_groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["name"] != "bash" {
		t.Fatalf("expected single bash group, got groups=%v", groups)
	}

	// The latest todo write (seq 3) wins over the earlier one (seq 1).
	if len(page.LatestTodos) != 3 {
		t.Fatalf("latest_todos: got %d, want 3 (most recent todowrite)", len(page.LatestTodos))
	}
	if page.LatestTodos[0]["content"] != "do thing 1" {
		t.Errorf("first todo content: got %v, want 'do thing 1'", page.LatestTodos[0]["content"])
	}
	if page.LatestTodos[1]["status"] != "in_progress" {
		t.Errorf("second todo status: got %v, want in_progress", page.LatestTodos[1]["status"])
	}
	if page.LatestTodos[2]["priority"] != "low" {
		t.Errorf("third todo priority: got %v, want low", page.LatestTodos[2]["priority"])
	}
}

func TestListMessages_ForeignOrgGets404(t *testing.T) {
	h := newMessagesListHarness(t)

	other := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("other-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := h.db.Create(&other).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", other.ID).Delete(&model.Org{}) })

	url := fmt.Sprintf("/v1/conversations/%s/messages", h.conv.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = middleware.WithOrg(req, &other)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for foreign org, got %d", rr.Code)
	}
}
