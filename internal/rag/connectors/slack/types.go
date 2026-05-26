package slack

import "fmt"

// SlackChannel mirrors the Slack conversations.list channel object.
// Fields match the exact API response documented at
// https://api.slack.com/methods/conversations.list
type SlackChannel struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	IsChannel         bool     `json:"is_channel"`
	IsGroup           bool     `json:"is_group"`
	IsIM              bool     `json:"is_im"`
	Created           int64    `json:"created"`
	Creator           string   `json:"creator"`
	IsArchived        bool     `json:"is_archived"`
	IsGeneral         bool     `json:"is_general"`
	Unlinked          int      `json:"unlinked"`
	NameNormalized    string   `json:"name_normalized"`
	IsShared          bool     `json:"is_shared"`
	IsExtShared       bool     `json:"is_ext_shared"`
	IsOrgShared       bool     `json:"is_org_shared"`
	IsMember          bool     `json:"is_member"`
	IsPrivate         bool     `json:"is_private"`
	IsMpim            bool     `json:"is_mpim"`
	Updated           int64    `json:"updated"`
	Topic             ChannelTopic `json:"topic"`
	Purpose           ChannelTopic `json:"purpose"`
	PreviousNames     []string `json:"previous_names,omitempty"`
	NumMembers        int      `json:"num_members"`
	Team              string   `json:"team,omitempty"`
	ContextTeamID     string   `json:"context_team_id,omitempty"`
	SharedTeamIDs     []string `json:"shared_team_ids,omitempty"`
}

type ChannelTopic struct {
	Value   string `json:"value"`
	Creator string `json:"creator"`
	LastSet int64  `json:"last_set"`
}

type conversationsListResponse struct {
	OK               bool             `json:"ok"`
	Channels         []SlackChannel   `json:"channels"`
	ResponseMetadata responseMetadata `json:"response_metadata"`
	Error            string           `json:"error,omitempty"`
}

// SlackMessage mirrors a single Slack message from conversations.history
// or conversations.replies. Verified against official API docs.
type SlackMessage struct {
	Type         string      `json:"type"`
	User         string      `json:"user,omitempty"`
	Text         string      `json:"text"`
	TS           string      `json:"ts"`
	ThreadTS     string      `json:"thread_ts,omitempty"`
	BotID        string      `json:"bot_id,omitempty"`
	AppID        string      `json:"app_id,omitempty"`
	BotProfile   *BotProfile `json:"bot_profile,omitempty"`
	Subtype      string      `json:"subtype,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	ParentUserID string      `json:"parent_user_id,omitempty"`
	ReplyCount   int         `json:"reply_count,omitempty"`
}

type BotProfile struct {
	ID      string `json:"id,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	Name    string `json:"name,omitempty"`
	Updated int64  `json:"updated,omitempty"`
	AppID   string `json:"app_id,omitempty"`
	TeamID  string `json:"team_id,omitempty"`
}

type Attachment struct {
	ServiceName string `json:"service_name,omitempty"`
	Text        string `json:"text,omitempty"`
	Fallback    string `json:"fallback,omitempty"`
	ThumbURL    string `json:"thumb_url,omitempty"`
	ThumbWidth  int    `json:"thumb_width,omitempty"`
	ThumbHeight int    `json:"thumb_height,omitempty"`
	ID          int    `json:"id,omitempty"`
}

type messagesResponse struct {
	OK               bool             `json:"ok"`
	Messages         []SlackMessage   `json:"messages"`
	HasMore          bool             `json:"has_more"`
	PinCount         int              `json:"pin_count"`
	ResponseMetadata responseMetadata `json:"response_metadata,omitempty"`
	Error            string           `json:"error,omitempty"`
}

// SlackUser mirrors user object from users.info
type SlackUser struct {
	ID       string      `json:"id"`
	TeamID   string      `json:"team_id"`
	Name     string      `json:"name"`
	RealName string      `json:"real_name"`
	Deleted  bool        `json:"deleted"`
	IsBot    bool        `json:"is_bot"`
	Profile  UserProfile `json:"profile"`
}

type UserProfile struct {
	DisplayName string `json:"display_name"`
	RealName    string `json:"real_name"`
	Email       string `json:"email,omitempty"`
	Team        string `json:"team,omitempty"`
}

type userInfoResponse struct {
	OK    bool      `json:"ok"`
	User  SlackUser `json:"user"`
	Error string    `json:"error,omitempty"`
}

type membersResponse struct {
	OK               bool             `json:"ok"`
	Members          []string         `json:"members"`
	ResponseMetadata responseMetadata `json:"response_metadata,omitempty"`
	Error            string           `json:"error,omitempty"`
}

type authTestResponse struct {
	OK           bool   `json:"ok"`
	URL          string `json:"url,omitempty"`
	Team         string `json:"team,omitempty"`
	User         string `json:"user,omitempty"`
	TeamID       string `json:"team_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	BotID        string `json:"bot_id,omitempty"`
	EnterpriseID string `json:"enterprise_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type responseMetadata struct {
	NextCursor string `json:"next_cursor"`
}

var disallowedMsgSubtypes = map[string]struct{}{
	"channel_join":               {},
	"channel_leave":              {},
	"channel_archive":            {},
	"channel_unarchive":          {},
	"pinned_item":                {},
	"unpinned_item":              {},
	"ekm_access_denied":          {},
	"channel_posting_permissions": {},
	"group_join":                 {},
	"group_leave":                {},
	"group_archive":              {},
	"group_unarchive":            {},
	"channel_name":               {},
}

const maxSlackPageSize = 200

func epochSeconds(ts string) int64 {
	var sec int64
	var frac int64
	_, err := fmt.Sscanf(ts, "%d.%d", &sec, &frac)
	if err != nil {
		return 0
	}
	return sec
}
