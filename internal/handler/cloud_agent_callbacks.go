package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type cloudAgentCallbackPayload struct {
	TaskID    string          `json:"task_id"`
	AgentID   string          `json:"agent_id"`
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Timestamp string          `json:"timestamp"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Data      json.RawMessage `json:"data"`
}

func cloudAgentCallbackPayloadFrom(task model.CloudAgentTask, event model.ConversationEvent) cloudAgentCallbackPayload {
	data := json.RawMessage(event.Data)
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}

	return cloudAgentCallbackPayload{
		TaskID:    task.ID.String(),
		AgentID:   task.CloudAgentID.String(),
		EventID:   event.EventID,
		EventType: event.EventType,
		Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
		Metadata:  map[string]any(task.Metadata),
		Data:      data,
	}
}

func dispatchCloudAgentCallback(ctx context.Context, db *gorm.DB, encKey *crypto.SymmetricKey, task model.CloudAgentTask, event model.ConversationEvent) error {
	if encKey == nil {
		return fmt.Errorf("encryption key is not configured")
	}

	var employeeSandbox model.Sandbox
	if err := db.WithContext(ctx).
		Where("agent_id = ? AND status NOT IN ?", task.EmployeeAgentID, []string{"archived", "archiving", "error"}).
		Order("created_at DESC").
		First(&employeeSandbox).Error; err != nil {
		return fmt.Errorf("load employee sandbox: %w", err)
	}
	if strings.TrimSpace(employeeSandbox.BridgeURL) == "" {
		return fmt.Errorf("employee sandbox bridge url is empty")
	}

	bridgeKey, err := encKey.DecryptString(employeeSandbox.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt employee bridge api key: %w", err)
	}

	body, err := json.Marshal(cloudAgentCallbackPayloadFrom(task, event))
	if err != nil {
		return fmt.Errorf("marshal cloud agent callback: %w", err)
	}

	url := strings.TrimRight(employeeSandbox.BridgeURL, "/") + "/gateway/cloud-agents/callback"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build callback request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bridgeKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send callback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("callback returned status %d", resp.StatusCode)
	}
	return nil
}
