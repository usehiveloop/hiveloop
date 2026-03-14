// cmd/fetchactions generates internal/mcp/catalog/actions.json from the Nango
// provider catalog. Each provider starts with an empty actions block. Actions
// for popular providers (Slack, GitHub, Google, Notion, Linear) are hand-curated
// after generation.
//
// Usage:
//
//	go run ./cmd/fetchactions              # fetch from Nango API (requires NANGO_ENDPOINT + NANGO_SECRET_KEY)
//	go run ./cmd/fetchactions <file.json>  # read provider list from local file
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
)

const outputPath = "internal/mcp/catalog/actions.json"

type providerActions struct {
	DisplayName string                `json:"display_name"`
	Actions     map[string]actionDef  `json:"actions"`
}

type actionDef struct {
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	ResourceType string          `json:"resource_type"`
	Parameters   json.RawMessage `json:"parameters,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fetchactions: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var providerNames []string

	if len(os.Args) > 1 {
		// Read from local file (JSON array of provider objects with "name" field).
		fmt.Printf("Reading from %s ...\n", os.Args[1])
		data, err := os.ReadFile(os.Args[1])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		var items []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &items); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}
		for _, item := range items {
			if item.Name != "" {
				providerNames = append(providerNames, item.Name)
			}
		}
	} else {
		// Fetch from Nango API.
		endpoint := os.Getenv("NANGO_ENDPOINT")
		secretKey := os.Getenv("NANGO_SECRET_KEY")
		if endpoint == "" || secretKey == "" {
			return fmt.Errorf("NANGO_ENDPOINT and NANGO_SECRET_KEY must be set")
		}

		fmt.Printf("Fetching providers from %s ...\n", endpoint)
		req, err := http.NewRequest("GET", endpoint+"/providers", nil)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetching providers: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		var items []struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
		for _, item := range items {
			if item.Name != "" {
				providerNames = append(providerNames, item.Name)
			}
		}
	}

	sort.Strings(providerNames)
	fmt.Printf("Found %d providers\n", len(providerNames))

	// Build catalog: empty skeletons for all providers
	catalog := make(map[string]providerActions, len(providerNames))
	for _, name := range providerNames {
		catalog[name] = providerActions{
			DisplayName: name,
			Actions:     map[string]actionDef{},
		}
	}

	// Overlay curated actions for popular providers
	overlayCuratedActions(catalog)

	out, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling catalog: %w", err)
	}

	if err := os.WriteFile(outputPath, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	stat, _ := os.Stat(outputPath)
	fmt.Printf("Wrote %s (%d KB, %d providers)\n", outputPath, stat.Size()/1024, len(catalog))
	return nil
}

func overlayCuratedActions(catalog map[string]providerActions) {
	// Slack
	catalog["slack"] = providerActions{
		DisplayName: "Slack",
		Actions: map[string]actionDef{
			"list_channels": {
				DisplayName:  "List Channels",
				Description:  "List all channels in the workspace",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","description":"Max results"}}}`),
			},
			"read_messages": {
				DisplayName:  "Read Messages",
				Description:  "Read messages from a channel",
				ResourceType: "channel",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","description":"Channel ID"},"limit":{"type":"integer","description":"Max messages"}},"required":["channel"]}`),
			},
			"send_message": {
				DisplayName:  "Send Message",
				Description:  "Send a message to a channel",
				ResourceType: "channel",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","description":"Channel ID"},"text":{"type":"string","description":"Message text"}},"required":["channel","text"]}`),
			},
		},
	}

	// GitHub
	catalog["github"] = providerActions{
		DisplayName: "GitHub",
		Actions: map[string]actionDef{
			"list_repos": {
				DisplayName:  "List Repositories",
				Description:  "List repositories for the authenticated user",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"per_page":{"type":"integer","description":"Results per page"},"sort":{"type":"string","description":"Sort field (created, updated, pushed, full_name)"}}}`),
			},
			"list_issues": {
				DisplayName:  "List Issues",
				Description:  "List issues in a repository",
				ResourceType: "repo",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"repo":{"type":"string","description":"Repository (owner/name)"},"state":{"type":"string","description":"Issue state (open, closed, all)"}},"required":["repo"]}`),
			},
			"create_issue": {
				DisplayName:  "Create Issue",
				Description:  "Create an issue in a repository",
				ResourceType: "repo",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"repo":{"type":"string","description":"Repository (owner/name)"},"title":{"type":"string","description":"Issue title"},"body":{"type":"string","description":"Issue body"}},"required":["repo","title"]}`),
			},
		},
	}

	// Google
	catalog["google"] = providerActions{
		DisplayName: "Google",
		Actions: map[string]actionDef{
			"list_files": {
				DisplayName:  "List Files",
				Description:  "List files in Google Drive",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"page_size":{"type":"integer","description":"Max results"},"q":{"type":"string","description":"Search query"}}}`),
			},
			"list_events": {
				DisplayName:  "List Events",
				Description:  "List calendar events",
				ResourceType: "calendar",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"calendar_id":{"type":"string","description":"Calendar ID"},"max_results":{"type":"integer","description":"Max events"}},"required":["calendar_id"]}`),
			},
		},
	}

	// Notion
	catalog["notion"] = providerActions{
		DisplayName: "Notion",
		Actions: map[string]actionDef{
			"search": {
				DisplayName:  "Search",
				Description:  "Search pages and databases in Notion",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"page_size":{"type":"integer","description":"Max results"}}}`),
			},
			"get_page": {
				DisplayName:  "Get Page",
				Description:  "Retrieve a Notion page",
				ResourceType: "page",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"page_id":{"type":"string","description":"Page ID"}},"required":["page_id"]}`),
			},
			"query_database": {
				DisplayName:  "Query Database",
				Description:  "Query a Notion database",
				ResourceType: "database",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"database_id":{"type":"string","description":"Database ID"},"page_size":{"type":"integer","description":"Max results"}},"required":["database_id"]}`),
			},
		},
	}

	// Linear
	catalog["linear"] = providerActions{
		DisplayName: "Linear",
		Actions: map[string]actionDef{
			"list_issues": {
				DisplayName:  "List Issues",
				Description:  "List issues in Linear",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"first":{"type":"integer","description":"Number of issues to fetch"}}}`),
			},
			"create_issue": {
				DisplayName:  "Create Issue",
				Description:  "Create an issue in Linear",
				ResourceType: "team",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"team_id":{"type":"string","description":"Team ID"},"title":{"type":"string","description":"Issue title"},"description":{"type":"string","description":"Issue description"}},"required":["team_id","title"]}`),
			},
			"list_projects": {
				DisplayName:  "List Projects",
				Description:  "List projects in Linear",
				ResourceType: "",
				Parameters:   json.RawMessage(`{"type":"object","properties":{"first":{"type":"integer","description":"Number of projects to fetch"}}}`),
			},
		},
	}
}
