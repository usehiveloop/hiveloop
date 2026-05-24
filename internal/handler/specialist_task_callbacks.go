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

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

const specialistCallbackTimeout = 15 * time.Second

type specialistCallbackPayload struct {
	TaskID       string          `json:"task_id"`
	SpecialistID string          `json:"specialist_id"`
	SessionID    string          `json:"session_id"`
	EventID      string          `json:"event_id"`
	EventType    string          `json:"event_type"`
	Timestamp    string          `json:"timestamp"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
	Data         json.RawMessage `json:"data"`
}

func specialistCallbackPayloadFrom(task model.SpecialistTask, event model.ConversationEvent) specialistCallbackPayload {
	data := json.RawMessage(event.Data)
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}

	return specialistCallbackPayload{
		TaskID:       task.ID.String(),
		SpecialistID: task.SpecialistID.String(),
		SessionID:    strings.TrimSpace(task.ParentConversationID),
		EventID:      event.EventID,
		EventType:    event.EventType,
		Timestamp:    event.Timestamp.UTC().Format(time.RFC3339),
		Metadata:     map[string]any(task.Metadata),
		Data:         data,
	}
}

type employeeCallbackSandboxSpecialists interface {
	NeedsURLRefresh(sb *model.Sandbox) bool
	RefreshEmployeeSandboxURL(ctx context.Context, sb *model.Sandbox) error
}

func dispatchSpecialistCallback(ctx context.Context, db *gorm.DB, encKey *crypto.SymmetricKey, runtime employeeCallbackSandboxSpecialists, task model.SpecialistTask, event model.ConversationEvent) error {
	if encKey == nil {
		return fmt.Errorf("encryption key is not configured")
	}
	if strings.TrimSpace(task.ParentConversationID) == "" {
		return fmt.Errorf("specialist task parent conversation id is empty")
	}

	var employeeSandbox model.Sandbox
	if err := db.WithContext(ctx).
		Where("employee_id = ? AND status NOT IN ?", task.EmployeeID, []string{"archived", "archiving", "error"}).
		Order("created_at DESC").
		First(&employeeSandbox).Error; err != nil {
		return fmt.Errorf("load employee sandbox: %w", err)
	}

	bridgeKey, err := encKey.DecryptString(employeeSandbox.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt employee bridge api key: %w", err)
	}

	if runtime != nil {
		if err := refreshEmployeeSandboxURLForCallback(ctx, runtime, &employeeSandbox); err != nil {
			return err
		}
	}
	body, err := json.Marshal(specialistCallbackPayloadFrom(task, event))
	if err != nil {
		return fmt.Errorf("marshal specialist callback: %w", err)
	}

	callbackCtx, cancel := context.WithTimeout(ctx, specialistCallbackTimeout)
	defer cancel()
	resp, err := sendSpecialistCallbackRequest(callbackCtx, employeeSandbox.BridgeURL, bridgeKey, body)
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

func refreshEmployeeSandboxURLForCallback(ctx context.Context, runtime employeeCallbackSandboxSpecialists, sb *model.Sandbox) error {
	if runtime.NeedsURLRefresh(sb) {
		if err := runtime.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			return fmt.Errorf("refresh employee sandbox URL for callback: %w", err)
		}
	}
	return nil
}

func sendSpecialistCallbackRequest(ctx context.Context, bridgeURL, bridgeKey string, body []byte) (*http.Response, error) {
	if strings.TrimSpace(bridgeURL) == "" {
		return nil, fmt.Errorf("employee sandbox bridge url is empty")
	}
	url := strings.TrimRight(bridgeURL, "/") + "/gateway/specialists/callback"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build callback request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bridgeKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	return client.Do(req)
}
