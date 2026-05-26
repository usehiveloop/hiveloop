package slack

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

const Kind = "slack"

var (
	_ interfaces.CheckpointedConnector[SlackCheckpoint] = (*SlackConnector)(nil)
	_ interfaces.PermSyncConnector                       = (*SlackConnector)(nil)
	_ interfaces.SlimConnector                           = (*SlackConnector)(nil)
)

// SlackConnector indexes Slack workspace messages via Nango-proxied
// Slack API calls. Only channels where the bot is already a member
// (is_member=true) are indexed — no automatic join is performed.
type SlackConnector struct {
	cfg    SlackConfig
	api    slackAPIClient
	userCache *userCache
	cleaner   *SlackTextCleaner

	channelBuf   int
	includeBots  bool
	workspaceURL string

	ctx       context.Context
	finalCp   atomic.Pointer[SlackCheckpoint]

	memberChannels []SlackChannel
}

func NewConnector(cfg SlackConfig, nangoClient *nango.Client, providerConfigKey, connectionID string) *SlackConnector {
	proxy := newSlackProxy(nangoClient, providerConfigKey, connectionID)
	uc := newUserCache()
	cleaner := newTextCleaner(proxy, uc)
	return &SlackConnector{
		cfg:        cfg,
		api:        proxy,
		userCache:  uc,
		cleaner:    cleaner,
		channelBuf: maxSlackPageSize * 2,
		includeBots: cfg.IncludeBotMessages,
	}
}

// newConnectorWithAPI is used in tests to inject a fake API client.
func newConnectorWithAPI(cfg SlackConfig, api slackAPIClient) *SlackConnector {
	uc := newUserCache()
	cleaner := newTextCleaner(api, uc)
	return &SlackConnector{
		cfg:         cfg,
		api:         api,
		userCache:   uc,
		cleaner:     cleaner,
		channelBuf:  maxSlackPageSize * 2,
		includeBots: cfg.IncludeBotMessages,
	}
}

func (c *SlackConnector) Kind() string { return Kind }

func (c *SlackConnector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func (c *SlackConnector) DummyCheckpoint() SlackCheckpoint {
	return dummyCheckpoint()
}

func (c *SlackConnector) UnmarshalCheckpoint(raw json.RawMessage) (SlackCheckpoint, error) {
	return unmarshalCheckpoint(raw)
}

// LoadFromCheckpoint starts (or resumes) a channel-by-channel backward
// traversal of Slack messages. The returned channel emits
// DocumentOrFailure items; it is closed when indexing completes.
func (c *SlackConnector) LoadFromCheckpoint(
	ctx context.Context,
	src interfaces.Source,
	cp SlackCheckpoint,
	start, end time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	c.ctx = ctx

	if cp.ChannelIDs == nil {
		if err := c.initWorkspace(ctx); err != nil {
			return nil, err
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			return nil, err
		}
		if len(channels) == 0 {
			cp.HasMore = false
			cp.ChannelCompletionMap = make(map[string]string)
			out := make(chan interfaces.DocumentOrFailure)
			close(out)
			return out, nil
		}
		c.memberChannels = channels
		cp.ChannelIDs = make([]string, len(channels))
		for i, ch := range channels {
			cp.ChannelIDs[i] = ch.ID
		}
		cp.ChannelCompletionMap = make(map[string]string)
		if len(channels) > 0 {
			cp.CurrentChannelID = channels[0].ID
			cp.CurrentChannelName = channels[0].Name
			cp.CurrentChannelIsPrivate = channels[0].IsPrivate
		}
		cp.HasMore = true
		if cp.WorkspaceURL == "" {
			cp.WorkspaceURL = c.workspaceURL
		}
	} else {
		c.workspaceURL = cp.WorkspaceURL
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			return nil, err
		}
		c.memberChannels = channels
	}

	if cp.ChannelCompletionMap == nil {
		cp.ChannelCompletionMap = make(map[string]string)
	}

	out := make(chan interfaces.DocumentOrFailure, c.channelBuf)
	go c.runIngest(ctx, cp, start, end, out)
	return out, nil
}

func (c *SlackConnector) initWorkspace(ctx context.Context) error {
	resp, err := c.api.authTest(ctx)
	if err != nil {
		return err
	}
	c.workspaceURL = resp.URL
	return nil
}

// fetchMemberChannels returns channels where is_member=true and are
// not archived. If channel_names is configured, further filters to
// only those channels. No automatic join is attempted.
func (c *SlackConnector) fetchMemberChannels(ctx context.Context) ([]SlackChannel, error) {
	all, err := c.api.listChannels(ctx, "public_channel,private_channel")
	if err != nil {
		return nil, err
	}
	filtered := make([]SlackChannel, 0)
	for _, ch := range all {
		if !ch.IsMember || ch.IsArchived {
			continue
		}
		if !channelIsAllowed(ch, c.cfg.ChannelNames, c.cfg.ChannelRegexEnabled) {
			continue
		}
		filtered = append(filtered, ch)
	}
	return filtered, nil
}

// ========== Build factory ==========

func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	connID, providerKey := connectionFromSource(src)
	return NewConnector(cfg, deps.Nango, providerKey, connID), nil
}

func connectionFromSource(src interfaces.Source) (string, string) {
	type connectionSource interface {
		NangoConnectionID() string
		NangoProviderConfigKey() string
	}
	if cs, ok := src.(connectionSource); ok {
		return cs.NangoConnectionID(), cs.NangoProviderConfigKey()
	}
	return "", Kind
}

func (c *SlackConnector) log(ctx context.Context, msg string, args ...interface{}) {
	logging.FromContext(ctx).InfoContext(ctx, msg, args...)
}
