package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	metadataPath = "internal/mcp/catalog/metadata.json"
	outputDir    = "internal/mcp/catalog/providers"
)

// ResourceDef mirrors the catalog ResourceDef.
type ResourceDef struct {
	DisplayName   string         `json:"display_name"`
	Description   string         `json:"description"`
	IDField       string         `json:"id_field"`
	NameField     string         `json:"name_field"`
	Icon          string         `json:"icon,omitempty"`
	ListAction    string         `json:"list_action"`
	RequestConfig *RequestConfig `json:"request_config,omitempty"`
}

// RequestConfig mirrors the catalog RequestConfig.
type RequestConfig struct {
	Method       string            `json:"method,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	QueryParams  map[string]string `json:"query_params,omitempty"`
	BodyTemplate map[string]any    `json:"body_template,omitempty"`
	ResponsePath string            `json:"response_path,omitempty"`
}

// ServiceMetadata holds hand-maintained data for a service.
type ServiceMetadata struct {
	DisplayName string                 `json:"display_name"`
	Resources   map[string]ResourceDef `json:"resources"`
}

// ProviderFile is the output format for each <provider>.actions.json file.
type ProviderFile struct {
	DisplayName string                 `json:"display_name"`
	Resources   map[string]ResourceDef `json:"resources"`
	Actions     map[string]ActionDef   `json:"actions"`
	Schemas     map[string]FlatSchema  `json:"schemas,omitempty"`
}

// loadMetadata reads the hand-maintained metadata.json.
func loadMetadata() (map[string]ServiceMetadata, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("reading metadata.json: %w", err)
	}
	var meta map[string]ServiceMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata.json: %w", err)
	}
	return meta, nil
}

// writeProviderFiles writes one .actions.json file per Nango provider ID.
func writeProviderFiles(cfg ServiceConfig, result *ParseResult, metadata map[string]ServiceMetadata) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Look up metadata for this service.
	meta, ok := metadata[cfg.Name]
	if !ok {
		meta = ServiceMetadata{
			DisplayName: cfg.Name,
			Resources:   map[string]ResourceDef{},
		}
	}

	pf := ProviderFile{
		DisplayName: meta.DisplayName,
		Resources:   meta.Resources,
		Actions:     result.Actions,
		Schemas:     result.Schemas,
	}

	out, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling provider file: %w", err)
	}

	for _, nangoID := range cfg.NangoProviders {
		filePath := filepath.Join(outputDir, nangoID+".actions.json")
		if err := os.WriteFile(filePath, out, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filePath, err)
		}
	}

	return nil
}
