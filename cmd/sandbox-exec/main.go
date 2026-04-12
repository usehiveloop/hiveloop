// One-shot tool for running shell commands inside a Daytona sandbox via the SDK.
//
// Usage:
//
//	go run ./cmd/sandbox-exec -id <sandbox-id> -cmd "echo hello"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

func main() {
	sandboxID := flag.String("id", "", "Sandbox ID")
	cmd := flag.String("cmd", "", "Command to execute")
	flag.Parse()

	if *sandboxID == "" || *cmd == "" {
		fmt.Fprintln(os.Stderr, "error: -id and -cmd are required")
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: os.Getenv("SANDBOX_PROVIDER_KEY"),
		APIUrl: os.Getenv("SANDBOX_PROVIDER_URL"),
		Target: os.Getenv("SANDBOX_TARGET"),
	})
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	defer client.Close(ctx)

	sandbox, err := client.Get(ctx, *sandboxID)
	if err != nil {
		log.Fatalf("get sandbox: %v", err)
	}

	result, err := sandbox.Process.ExecuteCommand(ctx, *cmd)
	if err != nil {
		log.Fatalf("exec: %v", err)
	}

	fmt.Print(result.Result)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
