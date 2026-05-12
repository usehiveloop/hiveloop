package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	slackprofile "github.com/usehiveloop/hiveloop/internal/profiles/slack"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

var _ interfaces.CheckpointedConnector[Checkpoint] = (*Connector)(nil)
var _ interfaces.SlimConnector = (*Connector)(nil)

type Connector struct {
	cfg      Config
	api      api
	identity slackprofile.Identity
	finalCp  atomic.Pointer[Checkpoint]
}

func NewConnector(cfg Config, api api, identity slackprofile.Identity) *Connector {
	return &Connector{cfg: cfg, api: api, identity: identity}
}

func Build(src interfaces.Source, deps interfaces.BuildDeps) (interfaces.Connector, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return nil, err
	}
	orgID, err := uuid.Parse(src.OrgID())
	if err != nil {
		return nil, fmt.Errorf("slack: invalid source org_id: %w", err)
	}
	secrets, identity, err := deps.ResolveSlackProfileSecrets(context.Background(), orgID, cfg.profileUUID())
	if err != nil {
		return nil, err
	}
	return NewConnector(cfg, newSDKAPI(secrets.BotToken), identity), nil
}

func (c *Connector) Kind() string { return Kind }

func (c *Connector) ValidateConfig(_ context.Context, src interfaces.Source) error {
	_, err := LoadConfig(src.Config())
	return err
}

func (c *Connector) DummyCheckpoint() Checkpoint { return dummyCheckpoint() }

func (c *Connector) UnmarshalCheckpoint(raw json.RawMessage) (Checkpoint, error) {
	return unmarshalCheckpoint(raw)
}

func (c *Connector) LoadFromCheckpoint(ctx context.Context, src interfaces.Source, cp Checkpoint, start, end time.Time) (<-chan interfaces.DocumentOrFailure, error) {
	if c.api == nil {
		return nil, fmt.Errorf("slack: api client is required")
	}
	if !cp.Stage.IsValid() {
		cp = dummyCheckpoint()
	}
	start = effectiveStart(start, c.cfg.HistoryDays)
	channels, err := c.accessibleChannels(ctx)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("slack: no accessible member channels")
	}
	out := make(chan interfaces.DocumentOrFailure, 32)
	go c.run(ctx, src, channels, start, end, out)
	return out, nil
}

func (c *Connector) Run(ctx context.Context, src interfaces.Source, checkpointJSON json.RawMessage, start, end time.Time) (<-chan interfaces.DocumentOrFailure, error) {
	cp, err := c.UnmarshalCheckpoint(checkpointJSON)
	if err != nil {
		return nil, err
	}
	return c.LoadFromCheckpoint(ctx, src, cp, start, end)
}

func (c *Connector) FinalCheckpoint() (json.RawMessage, error) {
	cp := c.finalCp.Load()
	if cp == nil {
		return nil, nil
	}
	return json.Marshal(*cp)
}

func (c *Connector) run(ctx context.Context, _ interfaces.Source, channels []channel, start, end time.Time, out chan<- interfaces.DocumentOrFailure) {
	defer close(out)
	defer func() {
		cp := Checkpoint{Stage: StageDone}
		c.finalCp.Store(&cp)
	}()

	for _, ch := range channels {
		select {
		case <-ctx.Done():
			out <- interfaces.NewDocFailure(interfaces.NewEntityFailure(ch.ID, "slack: ingest cancelled", &start, &end, ctx.Err()))
			return
		default:
		}
		docs, err := c.documentsForChannel(ctx, ch, start, end)
		if err != nil {
			out <- interfaces.NewDocFailure(interfaces.NewEntityFailure(ch.ID, "slack: fetch channel history", &start, &end, err))
			continue
		}
		for i := range docs {
			doc := docs[i]
			out <- interfaces.NewDocResult(&doc)
		}
	}
}

func (c *Connector) accessibleChannels(ctx context.Context) ([]channel, error) {
	seen := map[string]struct{}{}
	var out []channel
	if c.cfg.IncludePublicChannels {
		channels, err := c.listMemberChannels(ctx, []string{"public_channel"})
		if err != nil {
			return nil, err
		}
		for _, ch := range channels {
			if _, ok := seen[ch.ID]; ok {
				continue
			}
			seen[ch.ID] = struct{}{}
			out = append(out, ch)
		}
	}
	if c.cfg.IncludeJoinedPrivateChannels {
		channels, err := c.listMemberChannels(ctx, []string{"private_channel"})
		if err != nil {
			return nil, err
		}
		for _, ch := range channels {
			if _, ok := seen[ch.ID]; ok {
				continue
			}
			seen[ch.ID] = struct{}{}
			out = append(out, ch)
		}
	}
	return out, nil
}

func (c *Connector) listMemberChannels(ctx context.Context, types []string) ([]channel, error) {
	cursor := ""
	var out []channel
	for {
		channels, next, err := c.api.ListConversations(ctx, types, cursor)
		if err != nil {
			return nil, err
		}
		for _, ch := range channels {
			if ch.IsMember {
				out = append(out, ch)
			}
		}
		if next == "" {
			return out, nil
		}
		cursor = next
	}
}

func (c *Connector) documentsForChannel(ctx context.Context, ch channel, start, end time.Time) ([]interfaces.Document, error) {
	oldest := slackTime(start)
	latest := slackTime(end)
	cursor := ""
	var messages []message
	replies := map[string][]message{}
	for {
		page, next, err := c.api.History(ctx, ch.ID, oldest, latest, cursor)
		if err != nil {
			return nil, err
		}
		messages = append(messages, page...)
		for _, msg := range page {
			if msg.ReplyCount <= 0 || strings.TrimSpace(msg.Timestamp) == "" {
				continue
			}
			threadReplies, err := c.threadReplies(ctx, ch.ID, msg.Timestamp, oldest, latest)
			if err != nil {
				return nil, err
			}
			replies[msg.Timestamp] = threadReplies
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return documentsForChannelDay(ch, c.identity.TeamURL, c.identity.TeamID, c.cfg.AgentProfileID, messages, replies), nil
}

func (c *Connector) threadReplies(ctx context.Context, channelID, threadTS, oldest, latest string) ([]message, error) {
	cursor := ""
	var replies []message
	for {
		page, next, err := c.api.Replies(ctx, channelID, threadTS, oldest, latest, cursor)
		if err != nil {
			return nil, err
		}
		replies = append(replies, page...)
		if next == "" {
			return replies, nil
		}
		cursor = next
	}
}

func effectiveStart(start time.Time, historyDays int) time.Time {
	nowFloor := time.Now().UTC().AddDate(0, 0, -historyDays)
	if start.IsZero() || start.Before(nowFloor) {
		start = nowFloor
	}
	y, m, d := start.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func slackTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d.000000", t.UTC().Unix())
}

func (c *Connector) ListAllSlim(ctx context.Context, src interfaces.Source) (<-chan interfaces.SlimDocOrFailure, error) {
	stream, err := c.LoadFromCheckpoint(ctx, src, dummyCheckpoint(), time.Now().UTC().AddDate(0, 0, -c.cfg.HistoryDays), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	out := make(chan interfaces.SlimDocOrFailure, 32)
	go func() {
		defer close(out)
		for item := range stream {
			if item.Failure != nil {
				out <- interfaces.NewSlimFailure(item.Failure)
				continue
			}
			if item.Doc != nil {
				out <- interfaces.NewSlimResult(&interfaces.SlimDocument{DocID: item.Doc.DocID})
			}
		}
	}()
	return out, nil
}
