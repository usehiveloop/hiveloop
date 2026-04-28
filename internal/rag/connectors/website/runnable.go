package website

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

func (c *WebsiteConnector) Run(
	ctx context.Context,
	_ interfaces.Source,
	_ json.RawMessage,
	_, _ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	out := make(chan interfaces.DocumentOrFailure, 32)
	limit := c.cfg.MaxPages
	respect := true
	if c.cfg.RespectRobots != nil {
		respect = *c.cfg.RespectRobots
	}

	stream, errs := c.spider.CrawlStream(ctx, spider.SpiderParams{
		URL:           c.cfg.URL,
		ReturnFormat:  "markdown",
		RequestType:   "smart",
		Readability:   ptr(true),
		Sitemap:       ptr(true),
		RespectRobots: ptr(respect),
		Limit:         &limit,
	})

	go func() {
		defer close(out)
		for {
			select {
			case r, ok := <-stream:
				if !ok {
					if err, ok := <-errs; ok && err != nil {
						out <- interfaces.NewDocFailure(streamFailure(c.cfg.URL, err))
					}
					return
				}
				if r.Error != "" || (r.StatusCode != 0 && r.StatusCode >= 400) {
					out <- interfaces.NewDocFailure(pageFailure(r))
					continue
				}
				if strings.TrimSpace(r.Content) == "" {
					continue
				}
				doc := responseToDocument(r)
				out <- interfaces.NewDocResult(&doc)
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// FinalCheckpoint always returns nil — v1 has no resume; a failed crawl
// restarts from the seed URL on the next attempt.
func (c *WebsiteConnector) FinalCheckpoint() (json.RawMessage, error) {
	return nil, nil
}

func pageFailure(r spider.Response) *interfaces.ConnectorFailure {
	msg := r.Error
	if msg == "" {
		msg = "spider returned non-2xx status"
	}
	return interfaces.NewDocumentFailure(canonicalURL(r.URL), r.URL, msg, nil)
}

func streamFailure(seed string, err error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(seed, err.Error(), nil, nil, err)
}

func ptr[T any](v T) *T { return &v }
