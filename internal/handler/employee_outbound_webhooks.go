package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

var obviousSecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

type EmployeeOutboundWebhookHandler struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	memory   *hindsight.Client
	memCfg   hindsight.MemoryConfig
	now      func() time.Time
	maxBytes int64
}

type employeeOutboundEvent struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	At        time.Time       `json:"at"`
}

func NewEmployeeOutboundWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, memory *hindsight.Client) *EmployeeOutboundWebhookHandler {
	return &EmployeeOutboundWebhookHandler{
		db:       db,
		encKey:   encKey,
		memory:   memory,
		memCfg:   hindsight.DefaultMemoryConfig(),
		now:      time.Now,
		maxBytes: 512 * 1024,
	}
}

func (h *EmployeeOutboundWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return
	}
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !h.verifySignature(ctx, &sb, body, r.Header.Get("X-Hiveloop-Signature")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var event employeeOutboundEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}
	if event.At.IsZero() {
		event.At = h.now().UTC()
	}
	if h.memory != nil {
		h.retainEvent(ctx, &sb, &event)
	}
	h.db.WithContext(ctx).Model(&sb).Update("last_active_at", h.now())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *EmployeeOutboundWebhookHandler) verifySignature(ctx context.Context, sb *model.Sandbox, body []byte, signature string) bool {
	if h.encKey == nil {
		return true
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "employee outbound webhook: failed to decrypt runtime secret",
			"sandbox_id", sb.ID, "error", err)
		return false
	}
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "sha256="))
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (h *EmployeeOutboundWebhookHandler) retainEvent(ctx context.Context, sb *model.Sandbox, event *employeeOutboundEvent) {
	if sb.OrgID == nil || sb.AgentID == nil || h.memory == nil {
		return
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", *sb.AgentID).First(&agent).Error; err != nil {
		return
	}
	item, ok := buildEmployeeMemoryItem(sb, &agent, event)
	if !ok {
		return
	}
	bankID := hindsight.OrgBankID(*sb.OrgID)
	if err := h.memory.ConfigureBank(ctx, bankID, h.memCfg.ToBankConfigUpdate()); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee outbound webhook: configure memory bank bank_id=%s: %w", bankID, err))
		logging.FromContext(ctx).WarnContext(ctx, "employee outbound webhook: configure memory bank failed",
			"bank_id", bankID, "error", err)
	}
	if _, err := h.memory.Retain(ctx, bankID, &hindsight.RetainRequest{Items: []hindsight.RetainItem{item}, Async: true}); err != nil {
		logging.Capture(ctx, fmt.Errorf("employee outbound webhook: retain memory bank_id=%s event_type=%s: %w", bankID, event.EventType, err))
		logging.FromContext(ctx).WarnContext(ctx, "employee outbound webhook: retain failed",
			"bank_id", bankID, "event_type", event.EventType, "error", err)
	}
}

func buildEmployeeMemoryItem(sb *model.Sandbox, agent *model.Agent, event *employeeOutboundEvent) (hindsight.RetainItem, bool) {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	content := employeeMemoryContent(agent, event.EventType, payload)
	if shouldSkipMemoryContent(content, event.EventType) {
		return hindsight.RetainItem{}, false
	}
	sessionID := stringValue(payload, "session_id")
	channel := stringValue(payload, "channel")
	source := employeeEventSource(payload)
	tags := []string{
		"company:" + sb.OrgID.String(),
		"source:" + source,
		"memory_type:" + employeeEventMemoryType(agent),
	}
	visibility := "visibility:company"
	teamTag := ""
	if agent.TeamID != nil {
		teamTag = "team:" + agent.TeamID.String()
		visibility = "visibility:team"
	} else if strings.TrimSpace(agent.Team) != "" {
		teamTag = "team:" + strings.TrimSpace(agent.Team)
		visibility = "visibility:team"
	}
	if teamTag != "" {
		tags = append(tags, teamTag)
	}
	tags = append(tags, visibility)
	if channel != "" {
		tags = append(tags, "channel:"+channel)
	}
	observationScopes := [][]string{{"company:" + sb.OrgID.String()}}
	if teamTag != "" {
		observationScopes = append(observationScopes, []string{"company:" + sb.OrgID.String(), teamTag})
	}
	return hindsight.RetainItem{
		Content:           content,
		Context:           fmt.Sprintf("Employee outbound event from %s runtime", source),
		DocumentID:        employeeMemoryDocumentID(sb.ID, event, sessionID),
		Tags:              tags,
		Timestamp:         event.At.UTC().Format(time.RFC3339),
		Metadata:          employeeMemoryMetadata(sb, agent, event, payload),
		ObservationScopes: observationScopes,
	}, true
}

func employeeMemoryContent(agent *model.Agent, eventType string, payload map[string]any) string {
	switch eventType {
	case "user.message.received":
		speaker := stringValue(payload, "user_display_name")
		if speaker == "" {
			speaker = stringValue(payload, "user")
		}
		return fmt.Sprintf("Message to employee %s from %s: %s", agent.Name, speaker, stringValue(payload, "text"))
	case "agent.message.sent":
		return fmt.Sprintf("Employee %s replied: %s", agent.Name, stringValue(payload, "text"))
	case "tool.invoked":
		return fmt.Sprintf("Employee %s used tool %s. Result summary: %s", agent.Name, stringValue(payload, "tool"), stringValue(payload, "result_summary"))
	default:
		return ""
	}
}

func shouldSkipMemoryContent(content, eventType string) bool {
	content = strings.TrimSpace(content)
	if content == "" || strings.HasPrefix(eventType, "error.") {
		return true
	}
	lower := strings.ToLower(content)
	if lower == "hi" || lower == "hello" || lower == "thanks" || lower == "thank you" {
		return true
	}
	return obviousSecretPattern.MatchString(content)
}

func employeeEventSource(payload map[string]any) string {
	source := sanitizeTagValue(stringValue(payload, "source"))
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "gateway"))
	}
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "platform"))
	}
	if source == "" {
		return "manual"
	}
	return source
}

func sanitizeTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
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
	return strings.Trim(b.String(), "-_")
}

func employeeEventMemoryType(agent *model.Agent) string {
	if agent.TeamID != nil || strings.TrimSpace(agent.Team) != "" {
		return "team_context"
	}
	return "company_context"
}

func employeeMemoryDocumentID(sandboxID uuid.UUID, event *employeeOutboundEvent, sessionID string) string {
	var b bytes.Buffer
	b.WriteString("employee-event:")
	b.WriteString(sandboxID.String())
	b.WriteString(":")
	b.WriteString(event.EventType)
	b.WriteString(":")
	b.WriteString(sessionID)
	b.WriteString(":")
	b.WriteString(event.At.UTC().Format(time.RFC3339Nano))
	return b.String()
}

func employeeMemoryMetadata(sb *model.Sandbox, agent *model.Agent, event *employeeOutboundEvent, payload map[string]any) map[string]string {
	meta := map[string]string{
		"sandbox_id":  sb.ID.String(),
		"agent_id":    agent.ID.String(),
		"event_type":  event.EventType,
		"recorded_at": event.At.UTC().Format(time.RFC3339),
	}
	for _, key := range []string{"session_id", "source", "channel", "thread_ts", "user", "tool"} {
		if value := stringValue(payload, key); value != "" {
			meta[key] = value
		}
	}
	return meta
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
