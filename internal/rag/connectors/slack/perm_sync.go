package slack

import (
	"context"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// SyncDocPermissions streams the current ACL for every document in
// this source. Public channels get IsPublic=true; private channels
// get member emails from conversations.members.
func (c *SlackConnector) SyncDocPermissions(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.DocExternalAccessOrFailure, error) {
	c.ctx = ctx
	out := make(chan interfaces.DocExternalAccessOrFailure, c.channelBuf)
	go func() {
		defer close(out)
		if c.workspaceURL == "" {
			if err := c.initWorkspace(ctx); err != nil {
				out <- interfaces.NewAccessFailure(entityFailure(
					"workspace", "slack: auth.test", err,
				))
				return
			}
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			out <- interfaces.NewAccessFailure(entityFailure(
				"channels", "slack: list channels", err,
			))
			return
		}
		c.memberChannels = channels

		for _, ch := range channels {
			access := c.channelAccess(ch)
			if err := c.streamChannelDocAccess(ctx, ch, access, out); err != nil {
				out <- interfaces.NewAccessFailure(entityFailure(
					ch.ID, "slack: stream doc access "+ch.Name, err,
				))
			}
		}
	}()
	return out, nil
}

func (c *SlackConnector) streamChannelDocAccess(
	ctx context.Context,
	channel SlackChannel,
	access *interfaces.ExternalAccess,
	out chan<- interfaces.DocExternalAccessOrFailure,
) error {
	latest := ""
	for {
		messages, hasMore, err := c.api.getChannelHistory(ctx, channel.ID, "", latest)
		if err != nil {
			return err
		}
		for _, msg := range messages {
			if shouldFilter(msg, c.includeBots) != "" {
				continue
			}
			docID := docIDFromMessage(channel.ID, msg)
			out <- interfaces.NewAccessResult(&interfaces.DocExternalAccess{
				DocID:          docID,
				ExternalAccess: access,
			})
		}
		if !hasMore || len(messages) == 0 {
			break
		}
	}
	return nil
}

// SyncExternalGroups streams group definitions for Slack channels.
// Each private channel is emitted as an ExternalGroup with member
// emails so the scheduler can upsert them.
func (c *SlackConnector) SyncExternalGroups(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.ExternalGroupOrFailure, error) {
	c.ctx = ctx
	out := make(chan interfaces.ExternalGroupOrFailure, c.channelBuf)
	go func() {
		defer close(out)
		if c.workspaceURL == "" {
			if err := c.initWorkspace(ctx); err != nil {
				out <- interfaces.NewGroupFailure(entityFailure(
					"workspace", "slack: auth.test", err,
				))
				return
			}
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			out <- interfaces.NewGroupFailure(entityFailure(
				"channels", "slack: list channels", err,
			))
			return
		}
		c.memberChannels = channels

		for _, ch := range channels {
			if ch.IsPrivate {
				memberIDs, err := c.api.conversationMembers(ctx, ch.ID)
				if err != nil {
					out <- interfaces.NewGroupFailure(entityFailure(
						ch.ID, "slack: members "+ch.Name, err,
					))
					continue
				}
				emails := make([]string, 0, len(memberIDs))
				for _, memberID := range memberIDs {
					u, err := c.userCache.get(ctx, c.api, memberID)
					if err != nil || u == nil {
						continue
					}
					if email := userEmail(u); email != "" {
						emails = append(emails, email)
					}
				}
				out <- interfaces.NewGroupResult(&interfaces.ExternalGroup{
					GroupID:     "slack_channel_" + ch.ID,
					DisplayName: "#" + ch.Name,
					MemberEmails: emails,
				})
			}
		}
	}()
	return out, nil
}
