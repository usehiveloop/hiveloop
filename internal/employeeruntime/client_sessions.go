package employeeruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Session struct {
	ID             string    `json:"id"`
	Channel        string    `json:"channel"`
	ThreadTS       string    `json:"thread_ts"`
	AgentSessionID string    `json:"agent_session_id"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type ListSessionsParams struct {
	Cursor         string
	Status         string
	Limit          int
	SessionID      string
	Channel        string
	ThreadTS       string
	AgentSessionID string
	Search         string
}

type ListSessionsResponse struct {
	Items      []Session `json:"items"`
	NextCursor *string   `json:"next_cursor,omitempty"`
}

func (c *Client) ListSessions(ctx context.Context, params ListSessionsParams) (*ListSessionsResponse, error) {
	path := "/sessions"
	if encoded := sessionQuery(params).Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list sessions: %s: %s", resp.Status, raw)
	}
	var out ListSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode sessions response: %w", err)
	}
	return &out, nil
}

func sessionQuery(params ListSessionsParams) url.Values {
	values := url.Values{}
	if params.Cursor != "" {
		values.Set("cursor", params.Cursor)
	}
	if params.Status != "" {
		values.Set("status", params.Status)
	}
	if params.Limit > 0 {
		values.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.SessionID != "" {
		values.Set("session_id", params.SessionID)
	}
	if params.Channel != "" {
		values.Set("channel", params.Channel)
	}
	if params.ThreadTS != "" {
		values.Set("thread_ts", params.ThreadTS)
	}
	if params.AgentSessionID != "" {
		values.Set("agent_session_id", params.AgentSessionID)
	}
	if params.Search != "" {
		values.Set("q", params.Search)
	}
	return values
}
