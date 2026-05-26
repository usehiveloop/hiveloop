package slack

import (
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// docID builds the canonical document identifier.
// Format: {channel_id}__{thread_ts}
func docID(channelID, ts string) string {
	return fmt.Sprintf("%s__%s", channelID, ts)
}

// docIDFromMessage builds a doc ID where ts is the message's thread_ts
// (if it is a thread parent) or the message's own ts.
func docIDFromMessage(channelID string, msg SlackMessage) string {
	ts := msg.ThreadTS
	if ts == "" {
		ts = msg.TS
	}
	return docID(channelID, ts)
}

// shouldFilter returns a reason string if the message should be
// excluded from indexing, or empty string if it should be kept.
func shouldFilter(msg SlackMessage, includeBots bool) string {
	if !includeBots {
		if msg.BotID != "" || msg.AppID != "" {
			return "bot"
		}
	}
	if _, disallowed := disallowedMsgSubtypes[msg.Subtype]; disallowed {
		return "disallowed_subtype"
	}
	return ""
}

// threadToDoc converts a Slack thread into an interfaces.Document.
// Each message in the thread becomes one Section.
// Ported from Onyx's thread_to_doc at
// backend/onyx/connectors/slack/connector.py:274-349.
func (c *SlackConnector) threadToDoc(
	channel SlackChannel,
	messages []SlackMessage,
	cleaner *SlackTextCleaner,
) interfaces.Document {
	if len(messages) == 0 {
		return interfaces.Document{}
	}

	first := messages[0]
	ts := first.ThreadTS
	if ts == "" {
		ts = first.TS
	}

	senderName := c.resolveSenderName(first.User)
	channelName := channel.Name
	cleaned := cleaner.IndexClean(c.ctx, first.Text)
	snippet := truncate(cleaned, 50)
	semanticID := strings.ReplaceAll(
		fmt.Sprintf("%s in #%s: %s", senderName, channelName, snippet),
		"\n", " ",
	)

	sections := make([]interfaces.Section, 0, len(messages))
	for _, msg := range messages {
		sections = append(sections, interfaces.Section{
			Text:  cleaner.IndexClean(c.ctx, msg.Text),
			Link:  messagePermalink(c.workspaceURL, channel.ID, msg.TS),
		})
	}

	var docUpdatedAt *time.Time
	maxTS := maxTimestamp(messages)
	if maxTS > 0 {
		t := time.Unix(maxTS, 0).UTC()
		docUpdatedAt = &t
	}

	owners := c.resolveSenderEmails(messages)

	access := c.channelAccess(channel)
	doc := interfaces.Document{
		DocID:        docID(channel.ID, ts),
		SemanticID:   semanticID,
		Sections:     sections,
		DocUpdatedAt: docUpdatedAt,
		PrimaryOwners: owners,
		Metadata: map[string]string{
			"Channel":     channelName,
			"source_type": "slack",
		},
	}
	applyAccess(&doc, access)
	return doc
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func maxTimestamp(messages []SlackMessage) int64 {
	var maxVal float64
	for _, msg := range messages {
		if ts := epochSeconds(msg.TS); int64(ts) > int64(maxVal) {
			maxVal = float64(ts)
		}
	}
	return int64(maxVal)
}

func (c *SlackConnector) resolveSenderName(userID string) string {
	if userID == "" {
		return "Unknown"
	}
	name, err := c.userCache.get(c.ctx, c.api, userID)
	if err != nil || name == nil {
		return userID
	}
	n := userDisplayName(name)
	if n == "Unknown" || n == "" {
		return userID
	}
	return n
}

func (c *SlackConnector) resolveSenderEmails(messages []SlackMessage) []string {
	seen := make(map[string]struct{})
	emails := make([]string, 0)
	for _, msg := range messages {
		if msg.User == "" {
			continue
		}
		if _, ok := seen[msg.User]; ok {
			continue
		}
		seen[msg.User] = struct{}{}

		u, err := c.userCache.get(c.ctx, c.api, msg.User)
		if err != nil || u == nil {
			continue
		}
		if email := userEmail(u); email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}

func (c *SlackConnector) channelAccess(channel SlackChannel) *interfaces.ExternalAccess {
	if channel.IsPrivate {
		members, err := c.api.conversationMembers(c.ctx, channel.ID)
		if err != nil {
			return &interfaces.ExternalAccess{IsPublic: false}
		}
		emails := make([]string, 0, len(members))
		for _, memberID := range members {
			u, err := c.userCache.get(c.ctx, c.api, memberID)
			if err != nil || u == nil {
				continue
			}
			if email := userEmail(u); email != "" {
				emails = append(emails, email)
			}
		}
		return &interfaces.ExternalAccess{
			ExternalUserEmails: emails,
			IsPublic:           false,
		}
	}
	return &interfaces.ExternalAccess{IsPublic: true}
}

func applyAccess(d *interfaces.Document, a *interfaces.ExternalAccess) {
	if a == nil {
		return
	}
	d.IsPublic = a.IsPublic
	if len(a.ExternalUserGroupIDs) > 0 {
		d.ACL = append(d.ACL, a.ExternalUserGroupIDs...)
	}
	if len(a.ExternalUserEmails) > 0 {
		d.ACL = append(d.ACL, a.ExternalUserEmails...)
	}
}
