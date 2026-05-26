package slack

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// runIngest performs channel-by-channel backward traversal on a
// separate goroutine. The caller provides a channel to receive
// results; this function closes it when done.
func (c *SlackConnector) runIngest(
	ctx context.Context,
	cp SlackCheckpoint,
	start, end time.Time,
	out chan<- interfaces.DocumentOrFailure,
) {
	defer close(out)
	defer func() {
		final := cp
		c.finalCp.Store(&final)
	}()

	if cp.CurrentChannelID == "" && len(cp.ChannelIDs) > 0 {
		cp.CurrentChannelID = cp.ChannelIDs[0]
	}

	for cp.CurrentChannelID != "" {
		if ctx.Err() != nil {
			return
		}

		channel := c.findChannelByID(cp.CurrentChannelID)
		if channel == nil {
			cp.CurrentChannelID = c.nextChannel(cp)
			continue
		}

		cp.CurrentChannelName = channel.Name
		cp.CurrentChannelIsPrivate = channel.IsPrivate

		oldest := cp.ChannelCompletionMap[channel.ID]
		latest := fmt.Sprintf("%d", end.Unix())
		if start.IsZero() {
			oldest = ""
		} else if oldest == "" {
			oldest = fmt.Sprintf("%d", start.Unix())
		}

		done, advanceErr := c.ingestChannel(ctx, channel, &cp, oldest, latest, out)
		if advanceErr != nil {
			out <- interfaces.NewDocFailure(entityFailure(
				channel.ID, "slack: ingest channel "+channel.Name, advanceErr,
			))
		}
		if done {
			cp.CurrentChannelID = c.nextChannel(cp)
		}
	}
}

// ingestChannel fetches one page of messages and processes them.
// Returns (done=true, err) — done is true when the channel is fully ingested.
func (c *SlackConnector) ingestChannel(
	ctx context.Context,
	channel *SlackChannel,
	cp *SlackCheckpoint,
	oldest, latest string,
	out chan<- interfaces.DocumentOrFailure,
) (bool, error) {
	messages, hasMore, err := c.api.getChannelHistory(ctx, channel.ID, oldest, latest)
	if err != nil {
		return false, err
	}

	seenThreads := make(map[string]struct{})
	for _, ts := range cp.SeenThreadTS {
		seenThreads[ts] = struct{}{}
	}

	newOldest := latest
	if len(messages) > 0 {
		newOldest = messages[0].TS
	}

	for _, msg := range messages {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		reason := shouldFilter(msg, c.includeBots)
		if reason != "" {
			continue
		}

		if msg.ThreadTS != "" {
			threadTS := msg.ThreadTS
			if _, seen := seenThreads[threadTS]; seen {
				continue
			}
			seenThreads[threadTS] = struct{}{}

			thread, err := c.fetchCleanThread(ctx, channel, threadTS)
			if err != nil {
				out <- interfaces.NewDocFailure(docFailure(
					docID(channel.ID, threadTS),
					threadPermalink(c.workspaceURL, channel.ID, threadTS, threadTS),
					"slack: fetch thread: "+err.Error(), err,
				))
				continue
			}
			if len(thread) > 0 {
				doc := c.threadToDoc(*channel, thread, c.cleaner)
				out <- interfaces.NewDocResult(&doc)
			}
		} else {
			threadTS := msg.TS
			if _, seen := seenThreads[threadTS]; seen {
				continue
			}
			seenThreads[threadTS] = struct{}{}

			doc := c.threadToDoc(*channel, []SlackMessage{msg}, c.cleaner)
			if doc.DocID != "" {
				out <- interfaces.NewDocResult(&doc)
			}
		}
	}

	cp.ChannelCompletionMap[channel.ID] = newOldest
	cp.SeenThreadTS = mapKeys(seenThreads)

	return !hasMore, nil
}

func (c *SlackConnector) fetchCleanThread(
	ctx context.Context, channel *SlackChannel, threadTS string,
) ([]SlackMessage, error) {
	replies, err := c.api.getThreadReplies(ctx, channel.ID, threadTS)
	if err != nil {
		return nil, err
	}
	filtered := make([]SlackMessage, 0, len(replies))
	for _, msg := range replies {
		if shouldFilter(msg, c.includeBots) == "" {
			filtered = append(filtered, msg)
		}
	}
	return filtered, nil
}

func (c *SlackConnector) findChannelByID(id string) *SlackChannel {
	for i := range c.memberChannels {
		if c.memberChannels[i].ID == id {
			return &c.memberChannels[i]
		}
	}
	return nil
}

func (c *SlackConnector) nextChannel(cp SlackCheckpoint) string {
	currentIdx := -1
	for i, id := range cp.ChannelIDs {
		if id == cp.CurrentChannelID {
			currentIdx = i
			break
		}
	}
	if currentIdx < 0 || currentIdx+1 >= len(cp.ChannelIDs) {
		return ""
	}
	nextID := cp.ChannelIDs[currentIdx+1]
	ch := c.findChannelByID(nextID)
	if ch != nil {
		cp.CurrentChannelName = ch.Name
		cp.CurrentChannelIsPrivate = ch.IsPrivate
	}
	return nextID
}

func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// cleanMessageText strips leading/trailing whitespace and replaces
// non-breaking spaces for cleaner indexing.
func cleanMessageText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return text
}
