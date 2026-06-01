package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

type GatewaySlackPayload struct {
	ConnectionID   string `json:"connection_id"`
	OrgID          string `json:"org_id"`
	EmployeeID     string `json:"employee_id"`
	ChannelID      string `json:"channel_id"`
	ThreadTS       string `json:"thread_ts"`
	TeamID         string `json:"team_id,omitempty"`
	StreamURL      string `json:"stream_url"`
	RuntimeURL     string `json:"runtime_url"`
	RuntimeAPIKey  string `json:"runtime_api_key"`
	SessionID      string `json:"session_id"`
	RuntimeConvoID string `json:"runtime_conversation_id"`
	TraceID        string `json:"trace_id"`
	TurnID         string `json:"turn_id"`
	SenderID       string `json:"sender_id"`
	ActionToken    string `json:"action_token,omitempty"`
	NangoConnID    string `json:"nango_connection_id"`
	ProviderKey    string `json:"provider_config_key"`
}

func NewGatewaySlackTask(payload GatewaySlackPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway slack payload: %w", err)
	}
	return asynq.NewTask(
		TypeGatewaySlack,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(610*time.Second),
	), nil
}
