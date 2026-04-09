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

// ResourceDef mirrors the catalog ResourceDef for metadata.
type ResourceDef struct {
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	IDField       string            `json:"id_field"`
	NameField     string            `json:"name_field"`
	Icon          string            `json:"icon,omitempty"`
	ListAction    string            `json:"list_action"`
	RequestConfig *RequestConfig    `json:"request_config,omitempty"`
	RefBindings   map[string]string `json:"ref_bindings,omitempty"`
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

	// Build resources: from config if defined, otherwise from metadata.json.
	resources := map[string]ResourceDef{}
	displayName := cfg.Name

	if len(cfg.Resources) > 0 {
		for name, rc := range cfg.Resources {
			rd := ResourceDef{
				DisplayName: rc.DisplayName,
				Description: rc.Description,
				IDField:     rc.IDField,
				NameField:   rc.NameField,
				Icon:        rc.Icon,
				ListAction:  rc.ListAction,
			}
			if rc.ListRequestConfig != nil {
				rd.RequestConfig = rc.ListRequestConfig
			}
			if len(rc.RefBindings) > 0 {
				rd.RefBindings = rc.RefBindings
			}
			resources[name] = rd
		}
		// Use display name from metadata if available.
		if meta, ok := metadata[cfg.Name]; ok {
			displayName = meta.DisplayName
		}
	} else {
		meta, ok := metadata[cfg.Name]
		if ok {
			displayName = meta.DisplayName
			resources = meta.Resources
		}
	}

	pf := ProviderFile{
		DisplayName: displayName,
		Resources:   resources,
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
