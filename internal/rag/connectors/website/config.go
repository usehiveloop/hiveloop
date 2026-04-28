package website

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// defaultMaxPages caps a single crawl. Sites larger than this fall back
// to BFS-with-budget, which naturally prefers shallower (more central)
// pages. Sitemap discovery is enabled in the connector so curated URLs
// fill the budget first.
const defaultMaxPages = 500

type WebsiteConfig struct {
	URL           string `json:"url"`
	MaxPages      int    `json:"max_pages,omitempty"`
	RespectRobots *bool  `json:"respect_robots,omitempty"`
}

func LoadConfig(raw json.RawMessage) (WebsiteConfig, error) {
	cfg := WebsiteConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return WebsiteConfig{}, fmt.Errorf("website: parse config: %w", err)
		}
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return WebsiteConfig{}, fmt.Errorf("website: url is required")
	}
	if !strings.Contains(cfg.URL, "://") {
		cfg.URL = "https://" + cfg.URL
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" {
		return WebsiteConfig{}, fmt.Errorf("website: invalid url %q", cfg.URL)
	}
	if cfg.MaxPages < 0 {
		return WebsiteConfig{}, fmt.Errorf("website: max_pages must be >= 0")
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = defaultMaxPages
	}
	return cfg, nil
}
