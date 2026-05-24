// Required env: HIVY_DAYTONA_API_KEY, HIVY_DAYTONA_API_URL, HIVY_DAYTONA_TARGET.
//
// Usage:
//
//	buildtemplates sandbox-runtime -version=v0.0.1              [-size=...]
//	buildtemplates sandbox-runtime-specialist -version=v0.0.1   [-size=...]
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
	case "sandbox-runtime":
		runSandboxRuntime(ctx, args, runtimeEmployeeVariant)
	case "sandbox-runtime-specialist":
		runSandboxRuntime(ctx, args, runtimeSpecialistVariant)
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
  buildtemplates sandbox-runtime -version=v0.0.1              [-size=all|small,medium,large,xlarge]
  buildtemplates sandbox-runtime-specialist -version=v0.0.1   [-size=all|small,medium,large,xlarge]`)
}
