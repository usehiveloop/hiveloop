package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

const employeeSandboxImageRepo = "ghcr.io/usehivy/employee-sandbox"

func runEmployeeSandbox(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("employee-sandbox", flag.ExitOnError)
	version := fs.String("version", "", "Tag of usehivy/employee-sandbox already published to GHCR (required, e.g. v0.0.1)")
	size := fs.String("size", "all", "Snapshot sizes to register (small, medium, large, xlarge, all)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required (e.g. v0.0.1)")
		os.Exit(1)
	}
	targetSizes, err := resolveSizes(*size)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := registerEmployeeSandboxSnapshots(ctx, *version, targetSizes); err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Println("Done.")
}

func registerEmployeeSandboxSnapshots(ctx context.Context, version string, targetSizes []string) error {
	cleanVersion := strings.TrimPrefix(version, "v")
	dashedVersion := strings.ReplaceAll(cleanVersion, ".", "-")
	imageRef := fmt.Sprintf("%s:%s", employeeSandboxImageRepo, version)

	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: os.Getenv("SANDBOX_PROVIDER_KEY"),
		APIUrl: os.Getenv("SANDBOX_PROVIDER_URL"),
		Target: os.Getenv("SANDBOX_TARGET"),
	})
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	for _, sizeName := range targetSizes {
		size, ok := sizes[sizeName]
		if !ok {
			return fmt.Errorf("unknown size: %s", sizeName)
		}
		name := employeeSandboxSnapshotName(dashedVersion, size.Name)
		log.Printf("Registering Daytona snapshot %q from %s (cpu=%d, mem=%dGB, disk=%dGB)...",
			name, imageRef, size.CPU, size.Memory, size.Disk)

		_, logChan, err := client.Snapshot.Create(ctx, &types.CreateSnapshotParams{
			Name:  name,
			Image: imageRef,
			Resources: &types.Resources{
				CPU:    size.CPU,
				Memory: size.Memory,
				Disk:   size.Disk,
			},
		})
		if err != nil {
			return fmt.Errorf("creating snapshot %q: %w", name, err)
		}
		for line := range logChan {
			log.Printf("[%s] %s", name, line)
		}

		final, err := client.Snapshot.Get(ctx, name)
		if err != nil {
			return fmt.Errorf("re-fetching snapshot %q after build: %w", name, err)
		}
		if final.State != "active" {
			reason := ""
			if final.ErrorReason != nil {
				reason = *final.ErrorReason
			}
			return fmt.Errorf("snapshot %q ended in state %q: %s", name, final.State, reason)
		}
		log.Printf("✓ Snapshot %q ready (state=%s id=%s)", name, final.State, final.ID)
	}
	return nil
}

func employeeSandboxSnapshotName(dashedVersion, size string) string {
	return fmt.Sprintf("hivy-employee-sandbox-%s-%s-v1", dashedVersion, size)
}
