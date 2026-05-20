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
	var events []model.ConversationEvent
	if err := r.db.Where("conversation_id = ? AND event_type IN ?",
		convID, []string{"message_received", "response_completed"}).
		Find(&events).Error; err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "", nil
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].SequenceNumber < events[j].SequenceNumber
	})

	var buf strings.Builder
	for _, e := range events {
		var data map[string]any
		if len(e.Data) > 0 {
			_ = json.Unmarshal(e.Data, &data)
		}
		if data == nil {
			continue
		}

		switch e.EventType {
		case "message_received":
			content, _ := data["content"].(string)
			if content != "" {
				buf.WriteString("User: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		case "response_completed":
			content, _ := data["full_response"].(string)
			if content != "" {
				buf.WriteString("Assistant: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		}
	}

	return strings.TrimSpace(buf.String()), nil
}
