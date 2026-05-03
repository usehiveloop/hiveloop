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
	version := flag.String("version", "", "Image version tag — drives the GHCR tag and Daytona snapshot name. Bump independently of -bridge-version to invalidate Daytona's frozen snapshot mirror without changing the bridge binary (required, e.g. 1.0.0)")
	bridgeVersion := flag.String("bridge-version", "", "usehiveloop/bridge release tag installed into the image (required, e.g. v1.0.0). Independent of -version.")
	size := flag.String("size", "all", "Snapshot sizes to register (small, medium, large, xlarge, all)")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required")
		flag.Usage()
		os.Exit(1)
	}
	if *bridgeVersion == "" {
		fmt.Fprintln(os.Stderr, "error: -bridge-version is required")
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

	if err := buildAndPush(ctx, *version, *bridgeVersion, targetSizes); err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Println("Done.")
}
