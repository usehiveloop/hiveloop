package config

import (
	"net/url"
	"strings"
)

const defaultProxyOriginURL = "https://proxy.usehivy.com"

func (c *Config) ProxyOriginURL() string {
	if c == nil {
		return defaultProxyOriginURL
	}
	return normalizeProxyURL(c.ProxyHost)
}

func (c *Config) ProxyOpenAIBaseURL() string {
	origin := c.ProxyOriginURL()
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(origin, "/") + "/v1/proxy/v1"
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/proxy/v1"):
		parsed.Path = path
	case strings.HasSuffix(path, "/v1/proxy"):
		parsed.Path = path + "/v1"
	case path == "":
		parsed.Path = "/v1/proxy/v1"
	default:
		parsed.Path = path + "/v1/proxy/v1"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultProxyOriginURL
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return strings.TrimRight(parsed.String(), "/")
}
