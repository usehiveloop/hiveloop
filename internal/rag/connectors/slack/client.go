package slack

import (
	"context"
	"errors"
	"fmt"
	"time"

	slacksdk "github.com/slack-go/slack"
)

const slackPageSize = 200

type channel struct {
	ID        string
	Name      string
	IsPrivate bool
	IsMember  bool
}

type message struct {
	User            string
	Username        string
	Text            string
	Timestamp       string
	ThreadTimestamp string
	ReplyCount      int
	SubType         string
	Permalink       string
}

type api interface {
	ListConversations(ctx context.Context, conversationTypes []string, cursor string) ([]channel, string, error)
	History(ctx context.Context, channelID, oldest, latest, cursor string) ([]message, string, error)
	Replies(ctx context.Context, channelID, threadTS, oldest, latest, cursor string) ([]message, string, error)
}

type sdkAPI struct {
	client *slacksdk.Client
}

func newSDKAPI(botToken string) *sdkAPI {
	return &sdkAPI{client: slacksdk.New(botToken)}
}

func (a *sdkAPI) ListConversations(ctx context.Context, conversationTypes []string, cursor string) ([]channel, string, error) {
	var channels []slacksdk.Channel
	var next string
	err := withRateLimitRetry(ctx, func() error {
		var err error
		channels, next, err = a.client.GetConversationsContext(ctx, &slacksdk.GetConversationsParameters{
			Cursor:          cursor,
			ExcludeArchived: true,
			Limit:           slackPageSize,
			Types:           conversationTypes,
		})
		return err
	})
	if err != nil {
		return nil, "", err
	}
	out := make([]channel, 0, len(channels))
	for _, ch := range channels {
		out = append(out, channel{ID: ch.ID, Name: ch.Name, IsPrivate: ch.IsPrivate, IsMember: ch.IsMember})
	}
	return out, next, nil
}

func (a *sdkAPI) History(ctx context.Context, channelID, oldest, latest, cursor string) ([]message, string, error) {
	var resp *slacksdk.GetConversationHistoryResponse
	err := withRateLimitRetry(ctx, func() error {
		var err error
		resp, err = a.client.GetConversationHistoryContext(ctx, &slacksdk.GetConversationHistoryParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     slackPageSize,
			Oldest:    oldest,
			Latest:    latest,
			Inclusive: true,
		})
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return sdkMessages(resp.Messages), resp.ResponseMetaData.NextCursor, nil
}

func (a *sdkAPI) Replies(ctx context.Context, channelID, threadTS, oldest, latest, cursor string) ([]message, string, error) {
	var messages []slacksdk.Message
	var next string
	err := withRateLimitRetry(ctx, func() error {
		var err error
		messages, _, next, err = a.client.GetConversationRepliesContext(ctx, &slacksdk.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     slackPageSize,
			Oldest:    oldest,
			Latest:    latest,
			Inclusive: true,
		})
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return sdkMessages(messages), next, nil
}

func sdkMessages(in []slacksdk.Message) []message {
	out := make([]message, 0, len(in))
	for _, msg := range in {
		out = append(out, message{
			User:            msg.User,
			Username:        msg.Username,
			Text:            msg.Text,
			Timestamp:       msg.Timestamp,
			ThreadTimestamp: msg.ThreadTimestamp,
			ReplyCount:      msg.ReplyCount,
			SubType:         msg.SubType,
			Permalink:       msg.Permalink,
		})
	}
	return out
}

func withRateLimitRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		var rateLimited *slacksdk.RateLimitedError
		if !errors.As(err, &rateLimited) {
			return err
		}
		wait := rateLimited.RetryAfter
		if wait <= 0 {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("slack: rate limited after retries: %w", err)
}
