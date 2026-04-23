package daytona

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

func (d *Driver) GetEndpoint(ctx context.Context, externalID string, port int) (string, error) {
	url := fmt.Sprintf("%s/sandbox/%s/ports/%d/signed-preview-url?expiresInSeconds=%d",
		d.apiURL, externalID, port, signedURLTTLSeconds)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating signed URL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting signed URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("signed URL request failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding signed URL response: %w", err)
	}

	return result.URL, nil
}
