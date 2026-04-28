package spider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CrawlStream POSTs to /v1/crawl with Content-Type: application/jsonl, so
// Spider returns one Response per line as pages are discovered. Pages
// stream onto out as they arrive; errors land on errs. Both channels
// close when the upstream stream ends.
//
// The buffered Crawl method keeps existing callers (admin proxy) on the
// JSON-array response shape — switch only ingest paths that benefit
// from progressive emission.
func (client *Client) CrawlStream(ctx context.Context, params SpiderParams) (<-chan Response, <-chan error) {
	out := make(chan Response, 32)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		body, err := json.Marshal(params)
		if err != nil {
			errs <- fmt.Errorf("marshal: %w", err)
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			client.endpoint+"/v1/crawl", bytes.NewReader(body))
		if err != nil {
			errs <- err
			return
		}
		req.Header.Set("Authorization", "Bearer "+client.apiKey)
		req.Header.Set("Content-Type", "application/jsonl")
		req.Header.Set("Accept", "application/jsonl")

		resp, err := client.httpClient.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			errs <- fmt.Errorf("spider stream %d: %s", resp.StatusCode, string(preview))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var r Response
			if err := json.Unmarshal(line, &r); err != nil {
				errs <- fmt.Errorf("decode line: %w", err)
				continue
			}
			select {
			case out <- r:
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("read stream: %w", err)
		}
	}()

	return out, errs
}
