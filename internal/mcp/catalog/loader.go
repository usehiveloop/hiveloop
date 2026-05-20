package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
)

//go:embed providers/*.actions.json
var providersFS embed.FS

//go:embed providers/*.triggers.json
var triggersFS embed.FS

//go:embed providers/*.resources.json
var resourcesFS embed.FS

var (
	globalCatalog *Catalog
	initOnce      sync.Once
)

// Global returns the singleton catalog, parsing the embedded provider files on first call.
func Global() *Catalog {
	initOnce.Do(func() {
		globalCatalog = mustParse()
	})
	return globalCatalog
}

func mustParse() *Catalog {
	c := &Catalog{
		providers:          make(map[string]*ProviderActions),
		triggers:           make(map[string]*ProviderTriggers),
		subscribableByType: make(map[string]subscribableEntry),
		subscribableByProv: make(map[string]map[string]SubscribableResource),
	}

	c.parseProviders()
	c.parseTriggers()
	c.parseSubscribableResources()
	c.aliasGitHubAppCodeReviews()

	return c
}

func (c *Catalog) parseProviders() {
	entries, err := fs.ReadDir(providersFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded providers directory: " + err.Error())
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".actions.json") {
			continue
		}

		providerKey := strings.TrimSuffix(name, ".actions.json")
		data, err := fs.ReadFile(providersFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var pa ProviderActions
		if err := json.Unmarshal(data, &pa); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		c.providers[providerKey] = &pa
	}
}

func (c *Catalog) parseTriggers() {
	triggerEntries, err := fs.ReadDir(triggersFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded triggers directory: " + err.Error())
	}

	for _, entry := range triggerEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".triggers.json") {
			continue
		}

		providerKey := strings.TrimSuffix(name, ".triggers.json")
		data, err := fs.ReadFile(triggersFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var pt ProviderTriggers
		if err := json.Unmarshal(data, &pt); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		c.triggers[providerKey] = &pt
	}
}

func (c *Catalog) parseSubscribableResources() {
	resourceEntries, err := fs.ReadDir(resourcesFS, "providers")
	if err != nil {
		panic("catalog: failed to read embedded resources directory: " + err.Error())
	}

	for _, entry := range resourceEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".resources.json") {
			continue
		}

		data, err := fs.ReadFile(resourcesFS, "providers/"+name)
		if err != nil {
			panic("catalog: failed to read " + name + ": " + err.Error())
		}

		var psr ProviderSubscribableResources
		if err := json.Unmarshal(data, &psr); err != nil {
			panic("catalog: failed to parse " + name + ": " + err.Error())
		}

		if psr.Provider == "" {
			panic("catalog: " + name + " is missing the top-level \"provider\" field")
		}
		if _, exists := c.subscribableByProv[psr.Provider]; !exists {
			c.subscribableByProv[psr.Provider] = make(map[string]SubscribableResource)
		}

		for resourceType, def := range psr.Resources {
			if existing, clash := c.subscribableByType[resourceType]; clash {
				panic(fmt.Sprintf(
					"catalog: subscribable resource_type %q declared by both %q and %q — keys must be globally unique across providers",
					resourceType, existing.Provider, psr.Provider,
				))
			}
			c.subscribableByType[resourceType] = subscribableEntry{
				Provider: psr.Provider,
				Def:      def,
			}
			c.subscribableByProv[psr.Provider][resourceType] = def
		}
	}
}

func (c *Catalog) aliasGitHubAppCodeReviews() {
	if p, ok := c.providers["github-app"]; ok {
		c.providers["github-app-code-reviews"] = p
	}
	if r, ok := c.subscribableByProv["github-app"]; ok {
		c.subscribableByProv["github-app-code-reviews"] = r
	}
}
