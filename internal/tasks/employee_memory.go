package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const employeeMemoryRetainTimeout = 220 * time.Second

var memorySecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

var memoryFillerMessages = map[string]struct{}{
	"+1":             {},
	"ah":             {},
	"better":         {},
	"classic":        {},
	"closer":         {},
	"cool":           {},
	"exactly":        {},
	"fine":           {},
	"good":           {},
	"great":          {},
	"handy":          {},
	"hmm":            {},
	"lol":            {},
	"nice":           {},
	"ok":             {},
	"okay":           {},
	"one sec":        {},
	"please":         {},
	"ship":           {},
	"thanks":         {},
	"threading here": {},
	"ty":             {},
	"ugh":            {},
	"works locally":  {},
	"yep":            {},
	"yes":            {},
}

type EmployeeMemoryRetainHandler struct {
	db       *gorm.DB
	memory   *hindsight.Client
	enqueuer enqueue.TaskEnqueuer
}

func NewEmployeeMemoryRetainHandler(db *gorm.DB, memory *hindsight.Client, enqueuer enqueue.TaskEnqueuer) *EmployeeMemoryRetainHandler {
	return &EmployeeMemoryRetainHandler{db: db, memory: memory, enqueuer: enqueuer}
}

func (h *EmployeeMemoryRetainHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.memory == nil {
		return nil
	}
	var payload EmployeeMemoryRetainPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory retain payload: %w", err)
	}
	if payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil || strings.TrimSpace(payload.SessionID) == "" {
		return nil
	}

	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", payload.AgentID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee for memory retain: %w", err)
	}
	if agent.OrgID == nil {
		return nil
	}

	events, err := h.loadPendingEvents(ctx, payload)
	if err != nil {
		return err
	}
	item, ok := buildEmployeeRetainItem(&agent, payload, events)
	if !ok {
		return nil
	}

	bankID := hindsight.OrgBankID(*agent.OrgID)
	if err := h.memory.ConfigureBank(ctx, bankID, hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: configure bank %s: %w", bankID, err))
		return fmt.Errorf("configure memory bank: %w", err)
	}
	retainCtx, cancel := context.WithTimeout(ctx, employeeMemoryRetainTimeout)
	defer cancel()
	if _, err := h.memory.Retain(retainCtx, bankID, &hindsight.RetainRequest{Items: []hindsight.RetainItem{item}, Async: false}); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: retain bank_id=%s agent_id=%s: %w", bankID, agent.ID, err))
		return fmt.Errorf("retain employee memory: %w", err)
	}

	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).
		Model(&model.EmployeeMemoryEvent{}).
		Where("id IN ?", employeeMemoryEventIDs(events)).
		Update("retained_at", now).Error; err != nil {
		return fmt.Errorf("mark employee memory events retained: %w", err)
	}

	h.enqueueRefresh(ctx, payload.AgentID, payload.SandboxID)
	return nil
}

func (h *EmployeeMemoryRetainHandler) loadPendingEvents(ctx context.Context, payload EmployeeMemoryRetainPayload) ([]model.EmployeeMemoryEvent, error) {
	var events []model.EmployeeMemoryEvent
	if err := h.db.WithContext(ctx).
		Where("agent_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NULL",
			payload.AgentID, payload.SandboxID, payload.SessionID).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load employee memory events: %w", err)
	}
	return events, nil
}

func (h *EmployeeMemoryRetainHandler) enqueueRefresh(ctx context.Context, agentID, sandboxID uuid.UUID) {
	if h.enqueuer == nil {
		return
	}
	task, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{
		AgentID:   agentID,
		SandboxID: sandboxID,
		Reason:    "hindsight_retain",
	})
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.Unique(2*time.Minute),
		asynq.TaskID("employee-memory-refresh:"+agentID.String()),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: enqueue refresh: %w", err))
	}
}

func buildEmployeeRetainItem(agent *model.Agent, payload EmployeeMemoryRetainPayload, events []model.EmployeeMemoryEvent) (hindsight.RetainItem, bool) {
	if agent == nil || agent.OrgID == nil || len(events) == 0 {
		return hindsight.RetainItem{}, false
	}
	if employeeMemoryEventsContainSecret(events) {
		return hindsight.RetainItem{}, false
	}
	if !employeeMemoryEventsHaveWorkSignal(events) {
		return hindsight.RetainItem{}, false
	}
	digest := employeeMemoryRetentionDigest(agent.Name, events)
	if !meaningfulEmployeeMemoryTranscript(digest, events) {
		return hindsight.RetainItem{}, false
	}
	source := dominantEmployeeMemorySource(events)
	tags := employeeMemoryTags(agent, source)
	channel := firstEmployeePayloadString(events, "channel")
	if channel != "" {
		tags = append(tags, "channel:"+sanitizeMemoryTagValue(channel))
	}
	observationScopes := [][]string{{"company:" + agent.OrgID.String()}}
	if teamTag := employeeMemoryTeamTag(agent); teamTag != "" {
		observationScopes = append(observationScopes, []string{"company:" + agent.OrgID.String(), teamTag})
	}
	return hindsight.RetainItem{
		Content:           digest,
		Context:           fmt.Sprintf("Filtered employee memory digest from %s source. It intentionally omits routine tool use and transient task chatter; retain only durable company/team facts, decisions, preferences, ownership, policies, and reusable technical context.", source),
		DocumentID:        "employee-session:" + payload.SandboxID.String() + ":" + payload.SessionID,
		Tags:              tags,
		Timestamp:         events[0].EventAt.UTC().Format(time.RFC3339),
		Metadata:          employeeMemoryRetainMetadata(agent, payload, events),
		ObservationScopes: observationScopes,
	}, true
}

func employeeMemoryRetentionDigest(agentName string, events []model.EmployeeMemoryEvent) string {
	var lines []string
	for _, event := range events {
		payload := employeeMemoryPayload(event)
		switch event.EventType {
		case "user.message.received":
			speaker := firstPayloadString(payload, "user_display_name", "user")
			if speaker == "" {
				speaker = "teammate"
			}
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeEmployeeMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Teammate %s: %s", speaker, text))
			}
		case "agent.message.sent":
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeEmployeeMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Employee %s: %s", agentName, text))
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("Conversation for memory extraction. This omits raw tool calls, internal commands, and execution trace.\n")
	buf.WriteString("Retain durable preferences, decisions, ownership, policies, recurring workflows, and stable technical/company context. Do not retain ordinary task execution or status chatter.\n\n")
	for _, line := range lines {
		buf.WriteString("- ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func employeeMemoryEventsContainSecret(events []model.EmployeeMemoryEvent) bool {
	for _, event := range events {
		payload := employeeMemoryPayload(event)
		for _, key := range []string{"text", "message", "result_summary"} {
			if value := firstPayloadString(payload, key); value != "" && memorySecretPattern.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func employeeMemoryEventsHaveWorkSignal(events []model.EmployeeMemoryEvent) bool {
	for _, event := range events {
		if event.EventType != "tool.invoked" {
			continue
		}
		payload := employeeMemoryPayload(event)
		tool := firstPayloadString(payload, "tool")
		if strings.TrimSpace(tool) != "" {
			return true
		}
	}
	return false
}

func shouldIncludeEmployeeMemoryMessage(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || isEmployeeMemoryFiller(text) || memorySecretPattern.MatchString(text) {
		return false
	}
	return true
}

func isEmployeeMemoryFiller(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.Trim(normalized, ".!?:; \t\n\r")
	_, ok := memoryFillerMessages[normalized]
	return ok
}

func meaningfulEmployeeMemoryTranscript(transcript string, events []model.EmployeeMemoryEvent) bool {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" || memorySecretPattern.MatchString(transcript) {
		return false
	}
	lower := strings.ToLower(transcript)
	if lower == "hi" || lower == "hello" || lower == "thanks" || lower == "thank you" {
		return false
	}
	hasUser := false
	hasCheckpoint := false
	for _, event := range events {
		if event.EventType == "user.message.received" {
			hasUser = true
		}
		if event.EventType == "agent.message.sent" || event.EventType == "session.completed" {
			hasCheckpoint = true
		}
	}
	return hasUser && hasCheckpoint
}

func employeeMemoryTags(agent *model.Agent, source string) []string {
	tags := []string{
		"company:" + agent.OrgID.String(),
		"source:" + sanitizeMemoryTagValue(source),
	}
	if teamTag := employeeMemoryTeamTag(agent); teamTag != "" {
		tags = append(tags, teamTag, "visibility:team", "memory_type:team_context")
	} else {
		tags = append(tags, "visibility:company", "memory_type:company_context")
	}
	return tags
}

func employeeMemoryTeamTag(agent *model.Agent) string {
	if agent.TeamID != nil {
		return "team:" + agent.TeamID.String()
	}
	if strings.TrimSpace(agent.Team) != "" {
		return "team:" + sanitizeMemoryTagValue(agent.Team)
	}
	return ""
}

func dominantEmployeeMemorySource(events []model.EmployeeMemoryEvent) string {
	counts := map[string]int{}
	for _, event := range events {
		source := strings.TrimSpace(event.Source)
		if source == "" {
			source = "manual"
		}
		counts[source]++
	}
	type pair struct {
		source string
		count  int
	}
	pairs := make([]pair, 0, len(counts))
	for source, count := range counts {
		pairs = append(pairs, pair{source: source, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].source < pairs[j].source
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) == 0 {
		return "manual"
	}
	return pairs[0].source
}

func employeeMemoryRetainMetadata(agent *model.Agent, payload EmployeeMemoryRetainPayload, events []model.EmployeeMemoryEvent) map[string]string {
	meta := map[string]string{
		"agent_id":     agent.ID.String(),
		"sandbox_id":   payload.SandboxID.String(),
		"session_id":   payload.SessionID,
		"event_count":  fmt.Sprintf("%d", len(events)),
		"source_event": payload.SourceEvent,
	}
	for _, key := range []string{"source", "channel", "thread_ts", "user", "tool"} {
		if value := firstEmployeePayloadString(events, key); value != "" {
			meta[key] = value
		}
	}
	return meta
}

func firstEmployeePayloadString(events []model.EmployeeMemoryEvent, key string) string {
	for _, event := range events {
		if value := firstPayloadString(employeeMemoryPayload(event), key); value != "" {
			return value
		}
	}
	return ""
}

func employeeMemoryPayload(event model.EmployeeMemoryEvent) map[string]any {
	var payload map[string]any
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	if payload == nil {
		return map[string]any{}
	}
	return payload
}

func firstPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeMemoryTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "manual"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "manual"
	}
	return out
}

func employeeMemoryEventIDs(events []model.EmployeeMemoryEvent) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

type EmployeeMemoryRefreshHandler struct {
	db          *gorm.DB
	compileDeps employeeruntime.CompileDeps
}

func NewEmployeeMemoryRefreshHandler(db *gorm.DB, compileDeps employeeruntime.CompileDeps) *EmployeeMemoryRefreshHandler {
	return &EmployeeMemoryRefreshHandler{db: db, compileDeps: compileDeps}
}

func (h *EmployeeMemoryRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.compileDeps.EncKey == nil {
		return nil
	}
	var payload EmployeeMemoryRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee memory refresh payload: %w", err)
	}
	if payload.AgentID == uuid.Nil {
		return nil
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ? AND is_employee = ?", payload.AgentID, true).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load employee for memory refresh: %w", err)
	}
	sb, err := h.loadSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if sb == nil {
		return nil
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt employee runtime secret: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, h.compileDeps, &agent)
	if err != nil {
		return fmt.Errorf("compile employee config for memory refresh: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutConfig(ctx, def); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "employee memory refreshed",
		"agent_id", agent.ID,
		"sandbox_id", sb.ID,
		"reason", payload.Reason,
	)
	return nil
}

func (h *EmployeeMemoryRefreshHandler) loadSandbox(ctx context.Context, payload EmployeeMemoryRefreshPayload) (*model.Sandbox, error) {
	var sb model.Sandbox
	q := h.db.WithContext(ctx).Where("agent_id = ? AND status <> ?", payload.AgentID, "error")
	if payload.SandboxID != uuid.Nil {
		q = q.Where("id = ?", payload.SandboxID)
	}
	err := q.Order("created_at DESC").First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load employee sandbox for memory refresh: %w", err)
	}
	return &sb, nil
}
