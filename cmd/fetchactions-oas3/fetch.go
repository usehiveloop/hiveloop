package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

const cacheDir = "/tmp/fetchactions-cache"

// fetchSpec downloads a spec from the given URL, caching it locally.
// If a cached version exists with the same URL hash, it is returned directly.
// Pass force=true to skip the cache.
func fetchSpec(url string, force bool) ([]byte, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	cachePath := filepath.Join(cacheDir, hash)

	if !force {
		if data, err := os.ReadFile(cachePath); err == nil {
			return data, nil
		}
	}

	fmt.Printf("  Downloading %s ...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetching %s: status %d: %s", url, resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	// Some JSON specs (e.g. Twilio) contain invalid unicode escape sequences
	// like \uD83D that confuse libopenapi's YAML parser. Strip them.
	if len(data) > 0 && data[0] == '{' {
		re := regexp.MustCompile(`\\u[dD][89abAB][0-9a-fA-F]{2}(\\u[dD][c-fC-F][0-9a-fA-F]{2})?`)
		data = re.ReplaceAll(data, []byte{})
	}

	_ = os.WriteFile(cachePath, data, 0644)
	return data, nil
}
