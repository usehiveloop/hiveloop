// Package slack implements the Slack provider for AgentProfile.
package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

const Provider = "slack"

// Mirrors the bot scopes in apps/web/app/onboarding/slack-manifest.ts —
// keep the two lists in sync.
var RequiredBotScopes = []string{
	"app_mentions:read",
	"assistant:write",
	"channels:history",
	"channels:join",
	"channels:read",
	"chat:write",
	"commands",
	"emoji:read",
	"files:read",
	"files:write",
	"groups:history",
	"groups:read",
	"groups:write",
	"im:history",
	"im:read",
	"im:write",
	"mpim:history",
	"mpim:read",
	"mpim:write",
	"pins:read",
	"pins:write",
	"reactions:read",
	"reactions:write",
	"users:read",
	"users:read.email",
}

var RequiredAppTokenScopes = []string{
	"connections:write",
	"app_configurations:write",
	"authorizations:read",
}

type Secrets struct {
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
}

type Identity struct {
	TeamID         string   `json:"team_id"`
	TeamName       string   `json:"team_name"`
	TeamURL        string   `json:"team_url"`
	BotUserID      string   `json:"bot_user_id"`
	BotUsername    string   `json:"bot_username"`
	BotID          string   `json:"bot_id"`
	BotScopes      []string `json:"bot_scopes"`
	AppTokenScopes []string `json:"app_token_scopes,omitempty"`
}

type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived"`
	IsMember   bool   `json:"is_member"`
	Topic      string `json:"topic,omitempty"`
	NumMembers int    `json:"num_members,omitempty"`
}

// ValidationError messages are safe to surface to the frontend.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func ValidateTokenFormat(s Secrets) error {
	if strings.TrimSpace(s.BotToken) == "" {
		return &ValidationError{Msg: "bot_token is required"}
	}
	if !strings.HasPrefix(s.BotToken, "xoxb-") {
		return &ValidationError{Msg: "bot_token must start with \"xoxb-\""}
	}
	if strings.TrimSpace(s.AppToken) == "" {
		return &ValidationError{Msg: "app_token is required"}
	}
	if !strings.HasPrefix(s.AppToken, "xapp-") {
		return &ValidationError{Msg: "app_token must start with \"xapp-\""}
	}
	return nil
}

// VerifyAndIntrospect validates both tokens against Slack and returns the
// persistable identity. Slack returns granted scopes in X-OAuth-Scopes on
// every Web API response; we read it directly because slack-go doesn't
// surface response headers.
func VerifyAndIntrospect(ctx context.Context, s Secrets) (Identity, error) {
	if err := ValidateTokenFormat(s); err != nil {
		return Identity{}, err
	}

	authResp, botScopes, err := callAuthTest(ctx, s.BotToken)
	if err != nil {
		return Identity{}, &ValidationError{Msg: fmt.Sprintf("slack rejected bot_token: %v", err)}
	}
	if missing := missingScopes(botScopes, RequiredBotScopes); len(missing) > 0 {
		return Identity{}, &ValidationError{Msg: fmt.Sprintf(
			"bot token is missing required scopes: %s — re-install the Slack app from the manifest and copy the new bot token",
			strings.Join(missing, ", "),
		)}
	}

	appScopes, err := callConnectionsOpen(ctx, s.AppToken)
	if err != nil {
		return Identity{}, &ValidationError{Msg: fmt.Sprintf("slack rejected app_token: %v", err)}
	}
	if missing := missingScopes(appScopes, RequiredAppTokenScopes); len(missing) > 0 {
		return Identity{}, &ValidationError{Msg: fmt.Sprintf(
			"app token is missing required scopes: %s — generate a new app-level token with these scopes in your Slack app's Basic Information page",
			strings.Join(missing, ", "),
		)}
	}

	return Identity{
		TeamID:         authResp.TeamID,
		TeamName:       authResp.Team,
		TeamURL:        authResp.URL,
		BotUserID:      authResp.UserID,
		BotUsername:    authResp.User,
		BotID:          authResp.BotID,
		BotScopes:      botScopes,
		AppTokenScopes: appScopes,
	}, nil
}

type authTestResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	BotID  string `json:"bot_id"`
}

func callAuthTest(ctx context.Context, botToken string) (authTestResponse, []string, error) {
	body, header, err := slackPost(ctx, "https://slack.com/api/auth.test", botToken)
	if err != nil {
		return authTestResponse{}, nil, err
	}
	var resp authTestResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return authTestResponse{}, nil, fmt.Errorf("decode auth.test response: %w", err)
	}
	if !resp.OK {
		return resp, nil, fmt.Errorf("auth.test: %s", resp.Error)
	}
	return resp, parseScopesHeader(header.Get("X-OAuth-Scopes")), nil
}

func callConnectionsOpen(ctx context.Context, appToken string) ([]string, error) {
	body, header, err := slackPost(ctx, "https://slack.com/api/apps.connections.open", appToken)
	if err != nil {
		return nil, err
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode apps.connections.open response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("apps.connections.open: %s", resp.Error)
	}
	return parseScopesHeader(header.Get("X-OAuth-Scopes")), nil
}

func slackPost(ctx context.Context, url, token string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header, nil
}

func parseScopesHeader(h string) []string {
	if h == "" {
		return nil
	}
	parts := strings.Split(h, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func missingScopes(granted, required []string) []string {
	set := make(map[string]struct{}, len(granted))
	for _, s := range granted {
		set[s] = struct{}{}
	}
	var missing []string
	for _, s := range required {
		if _, ok := set[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}

func ListPublicChannels(ctx context.Context, botToken string) ([]Channel, error) {
	const pageSize = 200
	const maxChannels = 1000

	client := slacksdk.New(botToken)

	out := make([]Channel, 0, pageSize)
	cursor := ""
	for len(out) < maxChannels {
		params := &slacksdk.GetConversationsParameters{
			Cursor:          cursor,
			ExcludeArchived: true,
			Limit:           pageSize,
			Types:           []string{"public_channel"},
		}
		channels, next, err := client.GetConversationsContext(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("list slack channels: %w", err)
		}
		for _, ch := range channels {
			out = append(out, Channel{
				ID:         ch.ID,
				Name:       ch.Name,
				IsPrivate:  ch.IsPrivate,
				IsArchived: ch.IsArchived,
				IsMember:   ch.IsMember,
				Topic:      ch.Topic.Value,
				NumMembers: ch.NumMembers,
			})
			if len(out) >= maxChannels {
				break
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return out, nil
}

func EncodeSecrets(s Secrets) ([]byte, error) { return json.Marshal(s) }

func DecodeSecrets(b []byte) (Secrets, error) {
	var s Secrets
	if len(b) == 0 {
		return s, errors.New("empty secrets blob")
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("decode slack secrets: %w", err)
	}
	return s, nil
}
