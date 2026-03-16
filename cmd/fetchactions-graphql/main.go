// cmd/fetchactions-graphql generates per-provider action files from GraphQL APIs.
// Supports two modes:
//   - SDL: download a .graphql schema file and parse it (preferred)
//   - Introspection: send a live introspection query to the GraphQL endpoint
//
// Usage:
//
//	go run ./cmd/fetchactions-graphql                       # generate all GraphQL providers
//	go run ./cmd/fetchactions-graphql -provider linear      # generate one service
//	go run ./cmd/fetchactions-graphql -force                # bypass cache for SDL downloads
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	provider := flag.String("provider", "", "Generate actions for a single service (by name)")
	force := flag.Bool("force", false, "Force re-download of SDL files (bypass cache)")
	flag.Parse()

	if err := run(*provider, *force); err != nil {
		fmt.Fprintf(os.Stderr, "fetchactions-graphql: %v\n", err)
		os.Exit(1)
	}
}

func run(providerFilter string, force bool) error {
	metadata, err := loadMetadata()
	if err != nil {
		return err
	}

	services := AllServices()
	if providerFilter != "" {
		var filtered []ServiceConfig
		for _, svc := range services {
			if svc.Name == providerFilter {
				filtered = append(filtered, svc)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("unknown provider %q", providerFilter)
		}
		services = filtered
	}

	totalFiles := 0
	totalActions := 0

	for _, svc := range services {
		var actions map[string]ActionDef

		if svc.SchemaURL != "" {
			// SDL mode: download and parse the .graphql schema file.
			fmt.Printf("[%s] Fetching SDL schema from %s ...\n", svc.Name, svc.SchemaURL)
			sdlContent, err := fetchSDL(svc.SchemaURL, force)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR fetching SDL: %v (skipping)\n", svc.Name, err)
				continue
			}
			fmt.Printf("[%s] SDL downloaded (%d KB)\n", svc.Name, len(sdlContent)/1024)

			schema, err := parseSDL(sdlContent)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR parsing SDL: %v (skipping)\n", svc.Name, err)
				continue
			}
			fmt.Printf("[%s] Parsed %d query fields, %d mutation fields\n",
				svc.Name, len(schema.QueryFields), len(schema.MutationFields))

			actions = parseSDLToActions(schema, svc)
		} else if svc.IntrospectionURL != "" {
			// Introspection mode: live query.
			fmt.Printf("[%s] Running introspection at %s ...\n", svc.Name, svc.IntrospectionURL)
			schema, err := runIntrospection(svc.IntrospectionURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] ERROR: %v (skipping)\n", svc.Name, err)
				continue
			}
			fmt.Printf("[%s] Schema loaded (%d types)\n", svc.Name, len(schema.Types))

			actions = parseSchema(schema, svc)
		} else {
			fmt.Fprintf(os.Stderr, "[%s] ERROR: no SchemaURL or IntrospectionURL configured (skipping)\n", svc.Name)
			continue
		}

		if len(actions) == 0 {
			fmt.Fprintf(os.Stderr, "[%s] WARNING: no actions generated (skipping)\n", svc.Name)
			continue
		}

		if err := writeProviderFiles(svc, actions, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR writing: %v\n", svc.Name, err)
			continue
		}

		for _, id := range svc.NangoProviders {
			fmt.Printf("  %s.actions.json: %d actions\n", id, len(actions))
		}
		totalFiles += len(svc.NangoProviders)
		totalActions += len(actions)
	}

	fmt.Printf("\nTotal: %d files, %d unique actions\n", totalFiles, totalActions)
	return nil
}
