package evals

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type Evidence struct {
	Events       []model.EmployeeSessionEvent `json:"events"`
	Tasks        []model.SpecialistTask       `json:"tasks"`
	ToolCalls    []ToolCall                   `json:"tool_calls"`
	FinalText    string                       `json:"final_text"`
	LastEventAt  time.Time                    `json:"last_event_at"`
	FinalEventAt time.Time                    `json:"final_event_at"`
}

type ToolCall struct {
	Name    string          `json:"name"`
	Args    json.RawMessage `json:"args,omitempty"`
	EventAt time.Time       `json:"event_at"`
}

func BuildEvidence(events []model.EmployeeSessionEvent, tasks []model.SpecialistTask) Evidence {
	out := Evidence{Events: events, Tasks: tasks}
	for _, event := range events {
		if event.EventAt.After(out.LastEventAt) {
			out.LastEventAt = event.EventAt
		}
		switch event.EventType {
		case "agent.tool.call":
			if call, ok := toolCallFromPayload(event.Payload, event.EventAt); ok {
				out.ToolCalls = append(out.ToolCalls, call)
			}
		case "agent.message.sent":
			text := textFromPayload(event.Payload)
			if strings.TrimSpace(text) != "" {
				out.FinalText = text
				out.FinalEventAt = event.EventAt
			}
		}
	}
	return out
}

func toolCallFromPayload(raw model.RawJSON, at time.Time) (ToolCall, bool) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ToolCall{}, false
	}
	event, _ := payload["agent_event"].(map[string]any)
	if event == nil {
		event = payload
	}
	name, _ := event["tool"].(string)
	if strings.TrimSpace(name) == "" {
		name, _ = event["name"].(string)
	}
	if strings.TrimSpace(name) == "" {
		return ToolCall{}, false
	}
	args, _ := json.Marshal(event["args"])
	if string(args) == "null" {
		args = nil
	}
	return ToolCall{Name: name, Args: args, EventAt: at}, true
}

func textFromPayload(raw model.RawJSON) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if text, _ := payload["text"].(string); strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	event, _ := payload["agent_event"].(map[string]any)
	if event == nil {
		return ""
	}
	if text, _ := event["text"].(string); strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return ""
}

func specialistTaskIDs(tasks []model.SpecialistTask) []uuid.UUID {
	ids := make([]uuid.UUID, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	return ids
}
