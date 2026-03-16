package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const cacheDir = "/tmp/fetchactions-cache"

// fetchSpec downloads a spec from the given URL, caching it locally.
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

	_ = os.WriteFile(cachePath, data, 0644)
	return data, nil
}
