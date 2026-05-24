package config

import "strings"

const defaultRuntimeControlPlaneBaseURL = "https://api.usehivy.com"

func (c *Config) RuntimeControlPlaneBaseURL() string {
	if c != nil {
		if base := normalizeRuntimeBaseURL(c.APIWebhookBaseURL); base != "" {
			return base
		}
		if base := normalizeRuntimeBaseURL(c.SpecialistSandboxHost); base != "" {
			return base
		}
	}
	return defaultRuntimeControlPlaneBaseURL
}

func normalizeRuntimeBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	return "https://" + raw
}
