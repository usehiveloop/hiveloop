package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type SSEEvent struct {
	Type string
	Data json.RawMessage
}

type SSESubscriber struct {
	client *http.Client
}

func NewSSESubscriber(client *http.Client) *SSESubscriber {
	return &SSESubscriber{client: client}
}

func (s *SSESubscriber) Subscribe(ctx context.Context, streamURL, apiKey string) (<-chan SSEEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build SSE request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to SSE stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("SSE stream returned status %d", resp.StatusCode)
	}

	ch := make(chan SSEEvent, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		var eventType string
		var dataLines []string

		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}

			line := scanner.Text()

			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			} else if line == "" {
				if eventType != "" && len(dataLines) > 0 {
					data := json.RawMessage(strings.Join(dataLines, "\n"))
					select {
					case ch <- SSEEvent{Type: eventType, Data: data}:
					default:
					}
				}
				eventType = ""
				dataLines = nil
			}
		}
	}()

	return ch, nil
}
