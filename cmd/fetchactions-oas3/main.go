// cmd/fetchactions-oas3 generates per-provider action files from OpenAPI 3.x specs.
//
// Usage:
//
//	go run ./cmd/fetchactions-oas3                     # generate all OAS3 providers
//	go run ./cmd/fetchactions-oas3 -provider github    # generate one service
//	go run ./cmd/fetchactions-oas3 -force              # bypass spec cache
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
		fmt.Fprintf(os.Stderr, "fetchactions-oas3: %v\n", err)
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
		actions, err := parseSpec(specData, svc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR parsing: %v (skipping)\n", svc.Name, err)
			continue
		}

		if len(actions) == 0 {
			fmt.Fprintf(os.Stderr, "[%s] WARNING: no actions generated (skipping)\n", svc.Name)
			continue
		}

		// Validate actions.
		errs := validateActions(actions)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "[%s] VALIDATION: %s\n", svc.Name, e)
			}
		}

		if err := writeProviderFiles(svc, actions, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ERROR writing: %v\n", svc.Name, err)
			continue
		}

		resourceScoped := 0
		for _, a := range actions {
			if a.ResourceType != "" {
				resourceScoped++
			}
		}

		for _, id := range svc.NangoProviders {
			fmt.Printf("  %s.actions.json: %d actions (%d resource-scoped)\n", id, len(actions), resourceScoped)
		}
		totalFiles += len(svc.NangoProviders)
		totalActions += len(actions)
	}

	fmt.Printf("\nTotal: %d files, %d unique actions\n", totalFiles, totalActions)
	return nil
}

// validateActions checks generated actions for common issues.
func validateActions(actions map[string]ActionDef) []string {
	var errs []string
	for key, a := range actions {
		if a.DisplayName == "" {
			errs = append(errs, fmt.Sprintf("%s: empty display_name", key))
		}
		if a.Execution == nil {
			errs = append(errs, fmt.Sprintf("%s: missing execution config", key))
			continue
		}
		if a.Execution.Method == "" {
			errs = append(errs, fmt.Sprintf("%s: empty execution.method", key))
		}
		if a.Execution.Path == "" {
			errs = append(errs, fmt.Sprintf("%s: empty execution.path", key))
		}
	}
	return errs
}
