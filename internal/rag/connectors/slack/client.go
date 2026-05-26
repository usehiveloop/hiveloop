package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/usehivy/hivy/internal/nango"
)

// slackAPIClient is the interface for Slack API calls. Production uses
// slackProxy (Nango-backed); tests use fakeSlackAPIClient.
type slackAPIClient interface {
	authTest(ctx context.Context) (*authTestResponse, error)
	listChannels(ctx context.Context, types string) ([]SlackChannel, error)
	getChannelHistory(ctx context.Context, channelID string, oldest string, latest string) ([]SlackMessage, bool, error)
	getThreadReplies(ctx context.Context, channelID string, threadTS string) ([]SlackMessage, error)
	getUserInfo(ctx context.Context, userID string) (*SlackUser, error)
	conversationMembers(ctx context.Context, channelID string) ([]string, error)
}

// slackProxy wraps nango.RawProxyRequest for Slack API calls.
// The Nango server injects the OAuth bot token so the connector
// never sees or stores credentials.
type slackProxy struct {
	nango             *nango.Client
	providerConfigKey string
	connectionID      string
}

func newSlackProxy(n *nango.Client, providerConfigKey, connectionID string) *slackProxy {
	return &slackProxy{
		nango:             n,
		providerConfigKey: providerConfigKey,
		connectionID:      connectionID,
	}
}

func (p *slackProxy) get(ctx context.Context, path string, query url.Values) (int, []byte, error) {
	rawQuery := ""
	if len(query) > 0 {
		rawQuery = query.Encode()
	}
	resp, err := p.nango.RawProxyRequest(
		ctx, "GET", p.providerConfigKey, p.connectionID, path, rawQuery, nil, "",
	)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Body, nil
}

func (p *slackProxy) post(ctx context.Context, path string, body url.Values) (int, []byte, error) {
	encoded := body.Encode()
	resp, err := p.nango.RawProxyRequest(
		ctx, "POST", p.providerConfigKey, p.connectionID, path, encoded, nil,
		"application/x-www-form-urlencoded",
	)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Body, nil
}

func (p *slackProxy) getJSON(ctx context.Context, path string, query url.Values, dest interface{}) error {
	status, body, err := p.get(ctx, path, query)
	if err != nil {
		return fmt.Errorf("slack %s: %w", path, err)
	}
	if status >= 400 {
		return fmt.Errorf("slack %s: status %d: %s", path, status, trimBody(body, 256))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("slack %s: decode: %w", path, err)
	}
	return nil
}

func trimBody(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max])
}

// ========== Slack API methods ==========

func (p *slackProxy) authTest(ctx context.Context) (*authTestResponse, error) {
	var resp authTestResponse
	if err := p.getJSON(ctx, "/auth.test", nil, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack auth.test: %s", resp.Error)
	}
	return &resp, nil
}

func (p *slackProxy) listChannels(ctx context.Context, types string) ([]SlackChannel, error) {
	channels := make([]SlackChannel, 0)
	cursor := ""
	for {
		q := url.Values{}
		q.Set("types", types)
		q.Set("exclude_archived", "true")
		q.Set("limit", fmt.Sprintf("%d", maxSlackPageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp conversationsListResponse
		if err := p.getJSON(ctx, "/conversations.list", q, &resp); err != nil {
			return nil, err
		}
		if !resp.OK {
			return nil, fmt.Errorf("slack conversations.list: %s", resp.Error)
		}
		channels = append(channels, resp.Channels...)
		cursor = resp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return channels, nil
}

func (p *slackProxy) getChannelHistory(ctx context.Context, channelID string, oldest string, latest string) ([]SlackMessage, bool, error) {
	q := url.Values{}
	q.Set("channel", channelID)
	q.Set("limit", fmt.Sprintf("%d", maxSlackPageSize))
	if oldest != "" {
		q.Set("oldest", oldest)
	}
	if latest != "" {
		q.Set("latest", latest)
	}
	var resp messagesResponse
	if err := p.getJSON(ctx, "/conversations.history", q, &resp); err != nil {
		return nil, false, err
	}
	if !resp.OK {
		return nil, false, fmt.Errorf("slack conversations.history: %s", resp.Error)
	}
	return resp.Messages, resp.HasMore, nil
}

func (p *slackProxy) getThreadReplies(ctx context.Context, channelID string, threadTS string) ([]SlackMessage, error) {
	replies := make([]SlackMessage, 0)
	cursor := ""
	for {
		q := url.Values{}
		q.Set("channel", channelID)
		q.Set("ts", threadTS)
		q.Set("limit", fmt.Sprintf("%d", maxSlackPageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp messagesResponse
		if err := p.getJSON(ctx, "/conversations.replies", q, &resp); err != nil {
			return nil, err
		}
		if !resp.OK {
			return nil, fmt.Errorf("slack conversations.replies: %s", resp.Error)
		}
		replies = append(replies, resp.Messages...)
		cursor = resp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return replies, nil
}

func (p *slackProxy) getUserInfo(ctx context.Context, userID string) (*SlackUser, error) {
	q := url.Values{}
	q.Set("user", userID)
	var resp userInfoResponse
	if err := p.getJSON(ctx, "/users.info", q, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack users.info: %s", resp.Error)
	}
	return &resp.User, nil
}

func (p *slackProxy) conversationMembers(ctx context.Context, channelID string) ([]string, error) {
	memberIDs := make([]string, 0)
	cursor := ""
	for {
		q := url.Values{}
		q.Set("channel", channelID)
		q.Set("limit", fmt.Sprintf("%d", maxSlackPageSize))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp membersResponse
		if err := p.getJSON(ctx, "/conversations.members", q, &resp); err != nil {
			return nil, err
		}
		if !resp.OK {
			return nil, fmt.Errorf("slack conversations.members: %s", resp.Error)
		}
		memberIDs = append(memberIDs, resp.Members...)
		cursor = resp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return memberIDs, nil
}

// userCache memoizes users.info lookups to avoid redundant API calls.
type userCache struct {
	mu    sync.Mutex
	cache map[string]*SlackUser
}

func newUserCache() *userCache {
	return &userCache{cache: make(map[string]*SlackUser)}
}

func (uc *userCache) get(ctx context.Context, api slackAPIClient, userID string) (*SlackUser, error) {
	uc.mu.Lock()
	if u, ok := uc.cache[userID]; ok {
		uc.mu.Unlock()
		return u, nil
	}
	uc.mu.Unlock()

	u, err := api.getUserInfo(ctx, userID)
	if err != nil {
		return nil, err
	}

	uc.mu.Lock()
	uc.cache[userID] = u
	uc.mu.Unlock()

	return u, nil
}

func userDisplayName(u *SlackUser) string {
	if u == nil {
		return "Unknown"
	}
	if u.Profile.DisplayName != "" {
		return u.Profile.DisplayName
	}
	if u.RealName != "" {
		return u.RealName
	}
	if u.Name != "" {
		return u.Name
	}
	return u.ID
}

func userEmail(u *SlackUser) string {
	if u == nil {
		return ""
	}
	return u.Profile.Email
}

// messagePermalink builds the permalink URL to a Slack message.
// Format: {workspace_url}/archives/{channel_id}/p{ts_without_dot}
func messagePermalink(workspaceURL, channelID, ts string) string {
	tsWithoutDot := strings.ReplaceAll(ts, ".", "")
	return fmt.Sprintf("%s/archives/%s/p%s",
		strings.TrimRight(workspaceURL, "/"), channelID, tsWithoutDot)
}

// threadPermalink builds a permalink with thread_ts parameter.
func threadPermalink(workspaceURL, channelID, ts, threadTS string) string {
	base := messagePermalink(workspaceURL, channelID, ts)
	if threadTS != "" {
		base += "?thread_ts=" + threadTS
	}
	return base
}
