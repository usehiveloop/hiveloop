package handler

import "github.com/usehivy/hivy/internal/slackapp"

func toSlackChannelResponses(channels []slackapp.Channel) []slackChannelResponse {
	out := make([]slackChannelResponse, 0, len(channels))
	for _, ch := range channels {
		out = append(out, slackChannelResponse{
			ID:         ch.ID,
			Name:       ch.Name,
			IsPrivate:  ch.IsPrivate,
			IsArchived: ch.IsArchived,
			IsMember:   ch.IsMember,
			Topic:      ch.Topic,
			Purpose:    ch.Purpose,
			NumMembers: ch.NumMembers,
		})
	}
	return out
}
