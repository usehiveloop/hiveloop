package handler

import (
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type slackEvent struct {
	Type     string `json:"type"`
	TeamID   string `json:"team_id"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
	Subtype  string `json:"subtype"`
	UserName string `json:"user_name"`
}

type slackEventCallback struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	TeamID    string     `json:"team_id"`
	Event     slackEvent `json:"event"`
}

func isSlackProvider(conn *model.Connection) bool {
	return conn != nil && conn.Integration.Provider == slackapp.Provider
}
