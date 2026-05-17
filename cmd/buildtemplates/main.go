// Required env: SANDBOX_PROVIDER_KEY, SANDBOX_PROVIDER_URL, SANDBOX_TARGET.
// Bridge target additionally needs GHCR_USERNAME and GHCR_PAT.
//
// Usage:
//
//	buildtemplates bridge -version=1.0.0 -bridge-version=v1.0.0 [-size=...] [-bridge-binary=...] [-build-image=false]
//	buildtemplates employee-sandbox -version=v0.0.1             [-size=...]
package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	target := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	switch target {
	case "bridge":
		runBridge(ctx, args)
	case "employee-sandbox":
		runEmployeeSandbox(ctx, args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown target %q\n", target)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  buildtemplates bridge -version=1.0.0 -bridge-version=v1.0.0 [-size=all|small,medium,large,xlarge] [-bridge-binary=...] [-build-image=false] [-register-snapshots=false]
  buildtemplates employee-sandbox -version=v0.0.1             [-size=all|small,medium,large,xlarge]`)
}
