package main

import (
	"context"
	"fmt"
	"log"
	"os"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

func buildDaytona(ctx context.Context, flavor, bridgeVersion string, targetSizes []string) error {
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: os.Getenv("SANDBOX_PROVIDER_KEY"),
		APIUrl: os.Getenv("SANDBOX_PROVIDER_URL"),
		Target: os.Getenv("SANDBOX_TARGET"),
	})
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	var image *daytona.DockerImage
	switch flavor {
	case flavorBridge:
		image = buildBridgeImage(bridgeVersion)
	case flavorDevBox:
		image = buildDevBoxImage(bridgeVersion)
	default:
		return fmt.Errorf("unknown flavor: %s (valid: %s, %s)", flavor, flavorBridge, flavorDevBox)
	}
	log.Printf("Generated Dockerfile (flavor=%s):\n%s\n", flavor, image.Dockerfile())

	for _, sizeName := range targetSizes {
		size, ok := sizes[sizeName]
		if !ok {
			return fmt.Errorf("unknown size: %s", sizeName)
		}

		name := snapshotName(flavor, bridgeVersion, size.Name)
		log.Printf("Building snapshot %q (cpu=%d, mem=%dGB, disk=%dGB)...",
			name, size.CPU, size.Memory, size.Disk)

		resources := &types.Resources{
			CPU:    size.CPU,
			Memory: size.Memory,
			Disk:   size.Disk,
		}

		snapshot, logChan, err := client.Snapshot.Create(ctx, &types.CreateSnapshotParams{
			Name:      name,
			Image:     image,
			Resources: resources,
		})
		if err != nil {
			return fmt.Errorf("creating snapshot %q: %w", name, err)
		}

		for line := range logChan {
			log.Printf("[%s] %s", name, line)
		}

		log.Printf("Snapshot %q created successfully (id=%s)", name, snapshot.Name)
	}

	return nil
}
