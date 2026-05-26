package slack

import (
	"context"
	"regexp"
	"strings"
)

// SlackTextCleaner replaces Slack-specific markup with human-readable
// text before indexing. Ported from Onyx's SlackTextCleaner at
// backend/onyx/connectors/slack/utils.py:215-329.
type SlackTextCleaner struct {
	api        slackAPIClient
	userCache  *userCache
	nameCache  map[string]string
	emailCache map[string]string
}

func newTextCleaner(api slackAPIClient, uc *userCache) *SlackTextCleaner {
	return &SlackTextCleaner{
		api:        api,
		userCache:  uc,
		nameCache:  make(map[string]string),
		emailCache: make(map[string]string),
	}
}

// IndexClean applies all cleaning transformations for indexing.
func (c *SlackTextCleaner) IndexClean(ctx context.Context, text string) string {
	text = c.replaceUserIDs(ctx, text)
	text = c.replaceChannelRefs(text)
	text = c.replaceSpecialMentions(text)
	text = c.replaceSubteamRefs(text)
	return text
}

// replaceUserIDs replaces <@U12345> with @display_name.
// Falls back to the raw user ID if lookup fails.
func (c *SlackTextCleaner) replaceUserIDs(ctx context.Context, text string) string {
	re := regexp.MustCompile(`<@(.*?)>`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		userID := strings.TrimPrefix(strings.TrimSuffix(match, ">"), "<@")
		name := c.resolveUserName(ctx, userID)
		return "@" + name
	})
}

// replaceChannelRefs replaces <#C12345|channel-name> with #channel-name.
func (c *SlackTextCleaner) replaceChannelRefs(text string) string {
	re := regexp.MustCompile(`<#(.*?)\|(.*?)>`)
	return re.ReplaceAllString(text, "#$2")
}

// replaceSpecialMentions replaces <!channel>, <!here>, <!everyone>.
func (c *SlackTextCleaner) replaceSpecialMentions(text string) string {
	text = strings.ReplaceAll(text, "<!channel>", "@channel")
	text = strings.ReplaceAll(text, "<!here>", "@here")
	text = strings.ReplaceAll(text, "<!everyone>", "@everyone")
	return text
}

// replaceSubteamRefs replaces <!subteam^ID|@team-name> with @team-name.
func (c *SlackTextCleaner) replaceSubteamRefs(text string) string {
	re := regexp.MustCompile(`<!([^|]+)\|([^>]+)>`)
	return re.ReplaceAllString(text, "$2")
}

// resolveUserName returns the display name for a user ID.
// On failure, returns the user ID itself (matching Onyx's behavior).
func (c *SlackTextCleaner) resolveUserName(ctx context.Context, userID string) string {
	if name, ok := c.nameCache[userID]; ok {
		return name
	}
	u, err := c.userCache.get(ctx, c.api, userID)
	if err != nil {
		c.nameCache[userID] = userID
		return userID
	}
	name := userDisplayName(u)
	if name == "Unknown" {
		name = userID
	}
	c.nameCache[userID] = name
	return name
}

// resolveUserEmail resolves a user ID to their email.
func (c *SlackTextCleaner) resolveUserEmail(ctx context.Context, userID string) string {
	if email, ok := c.emailCache[userID]; ok {
		return email
	}
	u, err := c.userCache.get(ctx, c.api, userID)
	if err != nil {
		c.emailCache[userID] = ""
		return ""
	}
	email := userEmail(u)
	c.emailCache[userID] = email
	return email
}
