package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const sdlCacheDir = "/tmp/fetchactions-cache"

// fetchSDL downloads a .graphql SDL file, caching it locally.
func fetchSDL(url string, force bool) (string, error) {
	if err := os.MkdirAll(sdlCacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	cachePath := filepath.Join(sdlCacheDir, "sdl-"+hash)

	if !force {
		if data, err := os.ReadFile(cachePath); err == nil {
			return string(data), nil
		}
	}

	fmt.Printf("  Downloading %s ...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetching %s: status %d: %s", url, resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	_ = os.WriteFile(cachePath, data, 0644)
	return string(data), nil
}
