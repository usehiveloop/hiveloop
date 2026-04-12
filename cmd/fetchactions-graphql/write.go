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
	DisplayName         string            `json:"display_name"`
	Description         string            `json:"description"`
	IDField             string            `json:"id_field"`
	NameField           string            `json:"name_field"`
	Icon                string            `json:"icon,omitempty"`
	ListAction          string            `json:"list_action"`
	RequestConfig       *RequestConfig    `json:"request_config,omitempty"`
	RefBindings         map[string]string `json:"ref_bindings,omitempty"`
	ResourceKeyTemplate string            `json:"resource_key_template,omitempty"`
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

// ProviderFile is the output format for each <provider>.actions.json.
type ProviderFile struct {
	DisplayName string                       `json:"display_name"`
	Resources   map[string]ResourceDef       `json:"resources"`
	Actions     map[string]ActionDef         `json:"actions"`
	Schemas     map[string]SchemaDefinition  `json:"schemas,omitempty"`
}

// loadMetadata reads metadata.json.
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
func writeProviderFiles(cfg ServiceConfig, actions map[string]ActionDef, schemas map[string]SchemaDefinition, metadata map[string]ServiceMetadata) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	meta, ok := metadata[cfg.Name]
	if !ok {
		meta = ServiceMetadata{
			DisplayName: cfg.Name,
			Resources:   map[string]ResourceDef{},
		}
	}

	providerFile := ProviderFile{
		DisplayName: meta.DisplayName,
		Resources:   meta.Resources,
		Actions:     actions,
		Schemas:     schemas,
	}

	out, err := json.MarshalIndent(providerFile, "", "  ")
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
