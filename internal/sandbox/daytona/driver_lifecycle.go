package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func (d *Driver) SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error {
	url := fmt.Sprintf("%s/sandbox/%s/autostop/%d", d.apiURL, externalID, intervalMinutes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *Driver) SetAutoArchive(ctx context.Context, externalID string, intervalMinutes int) error {
	url := fmt.Sprintf("%s/sandbox/%s/autoarchive/%d", d.apiURL, externalID, intervalMinutes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	cmdBody, _ := json.Marshal(map[string]string{"command": command})
	execURL := fmt.Sprintf("%s/toolbox/%s/toolbox/process/execute", d.apiURL, externalID)
	execReq, err := http.NewRequestWithContext(ctx, http.MethodPost, execURL, bytes.NewReader(cmdBody))
	if err != nil {
		return "", err
	}
	execReq.Header.Set("Authorization", "Bearer "+d.apiKey)
	execReq.Header.Set("Content-Type", "application/json")

	execResp, err := (&http.Client{Timeout: 120 * time.Second}).Do(execReq)
	if err != nil {
		return "", fmt.Errorf("executing command: %w", err)
	}
	defer execResp.Body.Close()

	respBody, _ := io.ReadAll(execResp.Body)
	if execResp.StatusCode >= 400 {
		return "", fmt.Errorf("execute command failed (status %d): %s", execResp.StatusCode, respBody)
	}

	var result struct {
		ExitCode int    `json:"exitCode"`
		Result   string `json:"result"`
	}
	json.Unmarshal(respBody, &result)

	if result.ExitCode != 0 {
		return result.Result, fmt.Errorf("command exited with code %d: %s", result.ExitCode, result.Result)
	}
	return result.Result, nil
}

func mapState(state interface{}) sandbox.SandboxStatus {
	s := fmt.Sprintf("%v", state)
	switch s {
	case "started", "running":
		return sandbox.StatusRunning
	case "stopped":
		return sandbox.StatusStopped
	case "creating", "starting", "pending":
		return sandbox.StatusStarting
	case "archived":
		return sandbox.StatusArchived
	case "archiving":
		return sandbox.StatusArchiving
	case "error", "unknown":
		return sandbox.StatusError
	default:
		return sandbox.StatusError
	}
}
