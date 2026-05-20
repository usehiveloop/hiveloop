package nango

import (
	"context"
	"fmt"
	"net/http"

	"github.com/usehivy/hivy/internal/logging"
)

// ConnectSessionEndUser identifies the end user for a Nango connect session.
type ConnectSessionEndUser struct {
	ID string `json:"id"`
}

// CreateConnectSessionRequest is the payload for creating a connect session in Nango.
type CreateConnectSessionRequest struct {
	EndUser             ConnectSessionEndUser `json:"end_user"`
	AllowedIntegrations []string              `json:"allowed_integrations,omitempty"`
}

// ConnectSessionResponse represents the response from creating a connect session.
type ConnectSessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// CreateConnectSession creates a Nango connect session.
// POST /connect/sessions
func (c *Client) CreateConnectSession(ctx context.Context, req CreateConnectSessionRequest) (*ConnectSessionResponse, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/connect/sessions", req)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("nango create connect session: %w", err))
		return nil, err
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected connect session response: missing 'data' key")
	}

	token, _ := data["token"].(string)
	expiresAt, _ := data["expires_at"].(string)
	if token == "" {
		return nil, fmt.Errorf("unexpected connect session response: missing 'token'")
	}

	return &ConnectSessionResponse{Token: token, ExpiresAt: expiresAt}, nil
}

// CreateReconnectSessionRequest is the payload for reconnecting an existing connection.
type CreateReconnectSessionRequest struct {
	ConnectionID  string `json:"connection_id"`
	IntegrationID string `json:"integration_id"`
}

// CreateReconnectSession creates a Nango reconnect session for an existing connection.
// POST /connect/sessions/reconnect
func (c *Client) CreateReconnectSession(ctx context.Context, req CreateReconnectSessionRequest) (*ConnectSessionResponse, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/connect/sessions/reconnect", req)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("nango create reconnect session %s: %w", req.ConnectionID, err))
		return nil, err
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected reconnect session response: missing 'data' key")
	}

	token, _ := data["token"].(string)
	expiresAt, _ := data["expires_at"].(string)
	if token == "" {
		return nil, fmt.Errorf("unexpected reconnect session response: missing 'token'")
	}

	return &ConnectSessionResponse{Token: token, ExpiresAt: expiresAt}, nil
}
