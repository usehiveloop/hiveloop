package slackapp

import (
	"context"
	"fmt"

	slacksdk "github.com/slack-go/slack"
)

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
			out = append(out, channelFromSlack(ch))
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

func ListBotChannels(ctx context.Context, botToken string) ([]Channel, error) {
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
			Types:           []string{"public_channel", "private_channel"},
		}
		channels, next, err := client.GetConversationsContext(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("list bot slack channels: %w", err)
		}
		for _, ch := range channels {
			if !ch.IsMember {
				continue
			}
			out = append(out, channelFromSlack(ch))
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

// JoinChannel makes the bot join a public channel (conversations.join).
// Slack's API is idempotent — joining an already-joined channel succeeds.
// Private channels can't be joined via this method; the bot must be invited
// by an existing member.
func JoinChannel(ctx context.Context, botToken, channelID string) (Channel, error) {
	client := slacksdk.New(botToken)
	ch, _, _, err := client.JoinConversationContext(ctx, channelID)
	if err != nil {
		return Channel{}, fmt.Errorf("join slack channel: %w", err)
	}
	if ch == nil {
		return Channel{}, nil
	}
	return channelFromSlack(*ch), nil
}

func channelFromSlack(ch slacksdk.Channel) Channel {
	return Channel{
		ID:         ch.ID,
		Name:       ch.Name,
		IsPrivate:  ch.IsPrivate,
		IsArchived: ch.IsArchived,
		IsMember:   ch.IsMember,
		Topic:      ch.Topic.Value,
		Purpose:    ch.Purpose.Value,
		NumMembers: ch.NumMembers,
	}
}
