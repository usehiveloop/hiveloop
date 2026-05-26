package employeeruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ControlCommandsRequest struct {
	Workdir        string   `json:"workdir,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	StopOnError    bool     `json:"stop_on_error"`
	Commands       []string `json:"commands"`
}

type ControlCommandsResponse struct {
	OK      bool                   `json:"ok"`
	Results []ControlCommandResult `json:"results"`
}

type ControlCommandResult struct {
	Command   string `json:"command"`
	ExitCode  *int   `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
	Truncated bool   `json:"truncated"`
	Output    string `json:"output"`
}

func (c *Client) RunCommands(ctx context.Context, req ControlCommandsRequest) (*ControlCommandsResponse, error) {
	resp, err := c.do(ctx, http.MethodPost, "/control/commands", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("run runtime commands: %s: %s", resp.Status, raw)
	}
	var out ControlCommandsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode commands response: %w", err)
	}
	return &out, nil
}
