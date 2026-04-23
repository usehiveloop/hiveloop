// Command buildtemplates creates base sandbox snapshots with Bridge pre-installed.
//
// It builds templates of different sizes (small, medium, large, xlarge) and
// flavors (bridge, dev-box) on the configured sandbox provider.
//
// Usage:
//
//	go run ./cmd/buildtemplates -version 0.10.0
//	go run ./cmd/buildtemplates -version 0.10.0 -size small
//	go run ./cmd/buildtemplates -version 0.10.0 -flavor dev-box
//	go run ./cmd/buildtemplates -version 0.10.0 -flavor dev-box -size medium
//	go run ./cmd/buildtemplates -version 0.10.0 -provider daytona
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	version := flag.String("version", "", "Bridge version to install (required, e.g. 0.10.0)")
	provider := flag.String("provider", "daytona", "Sandbox provider (daytona)")
	flavor := flag.String("flavor", flavorBridge, "Image flavor to build (bridge, dev-box)")
	size := flag.String("size", "all", "Template size to build (small, medium, large, xlarge, all)")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required")
		flag.Usage()
		os.Exit(1)
	}

	switch *flavor {
	case flavorBridge, flavorDevBox:
	default:
		fmt.Fprintf(os.Stderr, "error: unknown flavor %q (valid: %s, %s)\n", *flavor, flavorBridge, flavorDevBox)
		os.Exit(1)
	}

	var targetSizes []string
	if *size == "all" {
		targetSizes = []string{"small", "medium", "large", "xlarge"}
	} else {
		for _, s := range strings.Split(*size, ",") {
			s = strings.TrimSpace(s)
			if _, ok := sizes[s]; !ok {
				fmt.Fprintf(os.Stderr, "error: unknown size %q (valid: small, medium, large, xlarge, all)\n", s)
				os.Exit(1)
			}
			targetSizes = append(targetSizes, s)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var err error
	switch *provider {
	case "daytona":
		err = buildDaytona(ctx, *flavor, *version, targetSizes)
	default:
		err = fmt.Errorf("unsupported provider: %s", *provider)
	}

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Println("All templates built successfully.")
}
