package hindsight

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// buildTranscript reconstructs the conversation from persisted events.
func (r *Retainer) buildTranscript(convID uuid.UUID) (string, error) {
	var events []model.EmployeeSessionEvent
	if err := r.db.Where("employee_session_id = ? AND event_type IN ?",
		convID, []string{"message_received", "response_completed", "user.message.received", "agent.message.sent"}).
		Find(&events).Error; err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "", nil
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].SequenceNumber == 0 && events[j].SequenceNumber == 0 {
			return events[i].EventAt.Before(events[j].EventAt)
		}
		return events[i].SequenceNumber < events[j].SequenceNumber
	})

	var buf strings.Builder
	for _, e := range events {
		var data map[string]any
		if len(e.Payload) > 0 {
			_ = json.Unmarshal(e.Payload, &data)
		}
		if data == nil {
			continue
		}

		switch e.EventType {
		case "message_received", "user.message.received":
			content, _ := data["content"].(string)
			if content == "" {
				content, _ = data["text"].(string)
			}
			if content != "" {
				buf.WriteString("User: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		case "response_completed", "agent.message.sent":
			content, _ := data["full_response"].(string)
			if content == "" {
				content, _ = data["text"].(string)
			}
			if content != "" {
				buf.WriteString("Assistant: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		}
	}

	return strings.TrimSpace(buf.String()), nil
}
