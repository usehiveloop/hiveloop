// cmd/fetchactions-oas2 generates per-provider action files from Swagger 2.0 specs.
//
// Usage:
//
//	go run ./cmd/fetchactions-oas2                     # generate all OAS2 providers
//	go run ./cmd/fetchactions-oas2 -provider slack     # generate one service
//	go run ./cmd/fetchactions-oas2 -force              # bypass spec cache
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	provider := flag.String("provider", "", "Generate actions for a single service (by name)")
	force := flag.Bool("force", false, "Force re-download of specs (bypass cache)")
	flag.Parse()

	if err := run(*provider, *force); err != nil {
		fmt.Fprintf(os.Stderr, "fetchactions-oas2: %v\n", err)
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
		fmt.Printf("[%s] Fetching spec...\n", svc.Name)
		specData, err := fetchSpec(svc.SpecSource, force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR: %v (skipping)\n", svc.Name, err)
			continue
		}
		fmt.Printf("[%s] Spec downloaded (%d KB)\n", svc.Name, len(specData)/1024)

		fmt.Printf("[%s] Parsing operations...\n", svc.Name)
		result, err := parseSpec(specData, svc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR parsing: %v (skipping)\n", svc.Name, err)
			continue
		}

		if len(result.Actions) == 0 {
			fmt.Fprintf(os.Stderr, "[%s] WARNING: no actions generated (skipping)\n", svc.Name)
			continue
		}

		if err := writeProviderFiles(svc, result, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR writing: %v\n", svc.Name, err)
			continue
		}

		for _, id := range svc.NangoProviders {
			fmt.Printf("  %s.actions.json: %d actions, %d schemas\n", id, len(result.Actions), len(result.Schemas))
		}
		totalFiles += len(svc.NangoProviders)
		totalActions += len(result.Actions)
	}

	fmt.Printf("\nTotal: %d files, %d unique actions\n", totalFiles, totalActions)
	return nil
}
