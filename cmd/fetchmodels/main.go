// cmd/fetchmodels fetches https://models.dev/api.json, trims it to essential
// fields, and writes the result to internal/registry/models.json for go:embed.
//
// Usage:
//
//	go run ./cmd/fetchmodels              # fetch from network
//	go run ./cmd/fetchmodels <file.json>  # read from local file
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
)

const (
	sourceURL  = "https://models.dev/api.json"
	outputPath = "internal/registry/models.json"
)

type rawProvider struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	API    string               `json:"api,omitempty"`
	Doc    string               `json:"doc,omitempty"`
	Models map[string]*rawModel `json:"models"`
}

type rawModel struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Family           string      `json:"family,omitempty"`
	Reasoning        bool        `json:"reasoning,omitempty"`
	ToolCall         bool        `json:"tool_call,omitempty"`
	StructuredOutput bool        `json:"structured_output,omitempty"`
	OpenWeights      bool        `json:"open_weights,omitempty"`
	Knowledge        string      `json:"knowledge,omitempty"`
	ReleaseDate      string      `json:"release_date,omitempty"`
	Modalities       *modalities `json:"modalities,omitempty"`
	Cost             *cost       `json:"cost,omitempty"`
	Limit            *limit      `json:"limit,omitempty"`
	Status           string      `json:"status,omitempty"`
}

type modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

type cost struct {
	Input  float64 `json:"input,omitempty"`
	Output float64 `json:"output,omitempty"`
}

type limit struct {
	Context int64 `json:"context,omitempty"`
	Output  int64 `json:"output,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fetchmodels: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var body []byte
	var err error

	if len(os.Args) > 1 {
		// Read from local file.
		fmt.Printf("Reading from %s ...\n", os.Args[1])
		body, err = os.ReadFile(os.Args[1])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
	} else {
		// Fetch from network.
		fmt.Printf("Fetching %s ...\n", sourceURL)
		resp, err := http.Get(sourceURL)
		if err != nil {
			return fmt.Errorf("fetching models: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
	}

	// Parse full API response (map of provider ID → provider object).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	var providers []rawProvider
	for _, data := range raw {
		var p rawProvider
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.ID == "" || len(p.Models) == 0 {
			continue
		}
		providers = append(providers, p)
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID < providers[j].ID
	})

	totalModels := 0
	for _, p := range providers {
		totalModels += len(p.Models)
	}

	fmt.Printf("Parsed %d providers, %d models\n", len(providers), totalModels)

	out, err := json.Marshal(providers)
	if err != nil {
		return fmt.Errorf("marshaling trimmed data: %w", err)
	}

	if err := os.WriteFile(outputPath, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	stat, _ := os.Stat(outputPath)
	fmt.Printf("Wrote %s (%d KB)\n", outputPath, stat.Size()/1024)
	return nil
}
