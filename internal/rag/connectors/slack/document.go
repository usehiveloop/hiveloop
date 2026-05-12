package slack

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

type transcriptMessage struct {
	Channel     channel
	Message     message
	IsThread    bool
	ParentTS    string
	TeamURL     string
	SlackTeamID string
	ProfileID   string
}

func documentsForChannelDay(ch channel, teamURL, teamID, profileID string, messages []message, replies map[string][]message) []interfaces.Document {
	byDate := map[string][]transcriptMessage{}
	for _, msg := range messages {
		if skipMessage(msg) {
			continue
		}
		date := dateForSlackTimestamp(msg.Timestamp)
		if date == "" {
			continue
		}
		byDate[date] = append(byDate[date], transcriptMessage{
			Channel: ch, Message: msg, TeamURL: teamURL, SlackTeamID: teamID, ProfileID: profileID,
		})
		for _, reply := range replies[msg.Timestamp] {
			if skipMessage(reply) || reply.Timestamp == msg.Timestamp {
				continue
			}
			replyDate := dateForSlackTimestamp(reply.Timestamp)
			if replyDate == "" {
				continue
			}
			byDate[replyDate] = append(byDate[replyDate], transcriptMessage{
				Channel: ch, Message: reply, IsThread: true, ParentTS: msg.Timestamp, TeamURL: teamURL, SlackTeamID: teamID, ProfileID: profileID,
			})
		}
	}
	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	docs := make([]interfaces.Document, 0, len(dates))
	for _, date := range dates {
		rows := byDate[date]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Message.Timestamp < rows[j].Message.Timestamp })
		docUpdatedAt := latestMessageTime(rows)
		docs = append(docs, interfaces.Document{
			DocID:        fmt.Sprintf("slack:%s:%s", ch.ID, date),
			SemanticID:   fmt.Sprintf("Slack #%s on %s", ch.Name, date),
			Link:         slackArchiveLink(teamURL, ch.ID, ""),
			Sections:     []interfaces.Section{{Title: fmt.Sprintf("#%s — %s", ch.Name, date), Text: renderTranscript(rows), Link: slackArchiveLink(teamURL, ch.ID, "")}},
			IsPublic:     true,
			DocUpdatedAt: docUpdatedAt,
			Metadata: map[string]string{
				"source":           "slack",
				"channel_id":       ch.ID,
				"channel_name":     ch.Name,
				"date":             date,
				"slack_team_id":    teamID,
				"agent_profile_id": profileID,
				"message_count":    strconv.Itoa(len(rows)),
				"visibility":       visibility(ch),
			},
		})
	}
	return docs
}

func renderTranscript(rows []transcriptMessage) string {
	var b strings.Builder
	for _, row := range rows {
		ts := timeForSlackTimestamp(row.Message.Timestamp)
		speaker := row.Message.User
		if speaker == "" {
			speaker = row.Message.Username
		}
		if speaker == "" {
			speaker = "unknown"
		}
		text := strings.TrimSpace(row.Message.Text)
		if text == "" {
			continue
		}
		prefix := ""
		if row.IsThread {
			prefix = fmt.Sprintf("thread reply to %s ", row.ParentTS)
		}
		if !ts.IsZero() {
			b.WriteString("[")
			b.WriteString(ts.UTC().Format("15:04"))
			b.WriteString("] ")
		}
		b.WriteString(prefix)
		b.WriteString(speaker)
		b.WriteString(": ")
		b.WriteString(text)
		if link := messageLink(row); link != "" {
			b.WriteString(" (")
			b.WriteString(link)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func skipMessage(msg message) bool {
	if strings.TrimSpace(msg.Text) == "" {
		return true
	}
	switch msg.SubType {
	case "", "bot_message", "file_share", "thread_broadcast":
		return false
	default:
		return true
	}
}

func dateForSlackTimestamp(ts string) string {
	t := timeForSlackTimestamp(ts)
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func timeForSlackTimestamp(ts string) time.Time {
	seconds, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Time{}
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * 1_000_000_000)
	return time.Unix(whole, nanos).UTC()
}

func latestMessageTime(rows []transcriptMessage) *time.Time {
	var latest time.Time
	for _, row := range rows {
		t := timeForSlackTimestamp(row.Message.Timestamp)
		if t.After(latest) {
			latest = t
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &latest
}

func visibility(ch channel) string {
	if ch.IsPrivate {
		return "private_channel_org_visible"
	}
	return "public_channel_org_visible"
}

func messageLink(row transcriptMessage) string {
	if row.Message.Permalink != "" {
		return row.Message.Permalink
	}
	return slackArchiveLink(row.TeamURL, row.Channel.ID, row.Message.Timestamp)
}

func slackArchiveLink(teamURL, channelID, ts string) string {
	teamURL = strings.TrimRight(strings.TrimSpace(teamURL), "/")
	if teamURL == "" || channelID == "" {
		return ""
	}
	if ts == "" {
		return teamURL + "/archives/" + channelID
	}
	return teamURL + "/archives/" + channelID + "/p" + strings.ReplaceAll(ts, ".", "")
}
