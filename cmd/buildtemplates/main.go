// Required env: GHCR_USERNAME, GHCR_PAT, SANDBOX_PROVIDER_KEY,
// SANDBOX_PROVIDER_URL, SANDBOX_TARGET. See `make build-templates` for usage.
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
	version := flag.String("version", "", "Image version tag (required, e.g. 0.10.0)")
	size := flag.String("size", "all", "Snapshot sizes to register (small, medium, large, xlarge, all)")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required")
		flag.Usage()
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

	if err := buildAndPush(ctx, *version, targetSizes); err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Println("Done.")
}
