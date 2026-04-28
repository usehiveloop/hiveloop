package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type scenarioYAML struct {
	Name         string                 `yaml:"name,omitempty"`
	Integrations []scenarioIntegration  `yaml:"integrations,omitempty"`
	Connections  []scenarioConnection   `yaml:"connections,omitempty"`
	Proxy        []scenarioFixture      `yaml:"proxy,omitempty"`
}

type scenarioIntegration struct {
	UniqueKey   string         `yaml:"unique_key"`
	Provider    string         `yaml:"provider"`
	DisplayName string         `yaml:"display_name,omitempty"`
	Credentials map[string]any `yaml:"credentials,omitempty"`
}

type scenarioConnection struct {
	ID                string         `yaml:"id"`
	ProviderConfigKey string         `yaml:"provider_config_key"`
	Provider          string         `yaml:"provider,omitempty"`
	Credentials       map[string]any `yaml:"credentials,omitempty"`
}

type scenarioFixture struct {
	Method  string            `yaml:"method,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Pattern string            `yaml:"path_pattern,omitempty"`
	Status  int               `yaml:"status,omitempty"`
	Body    any               `yaml:"body,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

func loadScenario(name string) (*scenarioYAML, error) {
	path := resolveScenarioPath(name)
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from a trusted operator/admin call.
	if err != nil {
		return nil, err
	}
	var sc scenarioYAML
	if err := yaml.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &sc, nil
}

func applyScenario(st *store, sc *scenarioYAML) {
	for _, i := range sc.Integrations {
		dn := i.DisplayName
		if dn == "" {
			dn = i.Provider
		}
		integ := &integration{
			UniqueKey:     i.UniqueKey,
			Provider:      i.Provider,
			DisplayName:   dn,
			Credentials:   i.Credentials,
			WebhookURL:    "https://fake-nango.local/webhook/" + i.UniqueKey,
			WebhookSecret: deriveWebhookSecret(i.UniqueKey),
		}
		if integ.Credentials == nil {
			integ.Credentials = map[string]any{}
		}
		st.putIntegration(integ)
	}
	for _, c := range sc.Connections {
		provider := c.Provider
		if provider == "" {
			provider = providerFromKey(st, c.ProviderConfigKey)
		}
		creds := c.Credentials
		if creds == nil {
			creds = map[string]any{}
		}
		st.putConnection(&connection{
			ID:                c.ID,
			ProviderConfigKey: c.ProviderConfigKey,
			Provider:          provider,
			Credentials:       creds,
			ConnectionConfig:  map[string]any{},
		})
	}
	fixtures := make([]proxyFixture, 0, len(sc.Proxy))
	for _, f := range sc.Proxy {
		status := f.Status
		if status == 0 {
			status = 200
		}
		fixtures = append(fixtures, proxyFixture{
			Method:  strings.ToUpper(f.Method),
			Path:    f.Path,
			Pattern: f.Pattern,
			Status:  status,
			Body:    f.Body,
			Headers: f.Headers,
		})
	}
	st.setFixtures(fixtures)
}

func resolveScenarioPath(name string) string {
	if filepath.IsAbs(name) || strings.Contains(name, "/") {
		return name
	}
	base := os.Getenv("FAKE_NANGO_SCENARIOS_DIR")
	if base == "" {
		base = "./scenarios"
	}
	return filepath.Join(base, name+".yaml")
}
