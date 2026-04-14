package drive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// Client talks to the sandbox drive endpoint.
// Authenticated with BRIDGE_CONTROL_PLANE_API_KEY (already in every sandbox).
type Client struct {
	endpoint string // ZIRALOOP_DRIVE_ENDPOINT
	apiKey   string // BRIDGE_CONTROL_PLANE_API_KEY
	http     *http.Client
}

type asset struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	DownloadURL string `json:"download_url,omitempty"`
}

type listResponse struct {
	Assets []asset `json:"assets"`
}

func NewClient(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   os.Getenv("BRIDGE_CONTROL_PLANE_API_KEY"),
		http:     &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) configured() bool {
	return c.endpoint != "" && c.apiKey != ""
}

// PullIfExists downloads a file from drive if it exists. Returns nil if not found.
func (c *Client) PullIfExists(destPath, filename string) error {
	if !c.configured() {
		return nil
	}

	assetInfo, err := c.findByFilename(filename)
	if err != nil || assetInfo == nil {
		return err
	}

	return c.download(assetInfo.ID, destPath)
}

// Upload uploads a local file to the agent's drive. Replaces existing file with same name.
func (c *Client) Upload(localPath, filename string) error {
	if !c.configured() {
		return fmt.Errorf("drive not configured (missing ZIRALOOP_DRIVE_ENDPOINT or BRIDGE_CONTROL_PLANE_API_KEY)")
	}

	// Delete existing file with same name
	existing, _ := c.findByFilename(filename)
	if existing != nil {
		c.delete(existing.ID)
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", c.endpoint+"/assets", &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) findByFilename(filename string) (*asset, error) {
	req, err := http.NewRequest("GET", c.endpoint+"/assets", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var listResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, err
	}

	for _, assetItem := range listResp.Assets {
		if assetItem.Filename == filename {
			return &assetItem, nil
		}
	}
	return nil, nil
}

func (c *Client) download(assetID, destPath string) error {
	req, err := http.NewRequest("GET", c.endpoint+"/assets/"+assetID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get asset failed: HTTP %d", resp.StatusCode)
	}

	var assetResp asset
	if err := json.NewDecoder(resp.Body).Decode(&assetResp); err != nil {
		return err
	}

	if assetResp.DownloadURL == "" {
		return fmt.Errorf("no download URL in response")
	}

	dlResp, err := c.http.Get(assetResp.DownloadURL)
	if err != nil {
		return err
	}
	defer dlResp.Body.Close()

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, dlResp.Body)
	return err
}

func (c *Client) delete(assetID string) {
	req, _ := http.NewRequest("DELETE", c.endpoint+"/assets/"+assetID, nil)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.http.Do(req)
}
