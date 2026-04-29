package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

const ghcrRegistry = "ghcr.io"
const ghcrNamespace = "usehiveloop"

func buildAndPush(ctx context.Context, flavor, bridgeVersion string, targetSizes []string) error {
	user := os.Getenv("GHCR_USERNAME")
	pat := os.Getenv("GHCR_PAT")
	if user == "" || pat == "" {
		return fmt.Errorf("GHCR_USERNAME and GHCR_PAT environment variables are required")
	}

	if err := dockerLogin(ctx, user, pat); err != nil {
		return fmt.Errorf("docker login: %w", err)
	}

	var dockerfile string
	switch flavor {
	case flavorBridge:
		dockerfile = buildBridgeImage(bridgeVersion).Dockerfile()
	case flavorDevBox:
		dockerfile = buildDevBoxImage(bridgeVersion).Dockerfile()
	default:
		return fmt.Errorf("unknown flavor: %s (valid: %s, %s)", flavor, flavorBridge, flavorDevBox)
	}

	tmpDir, err := os.MkdirTemp("", "buildtemplates-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}
	log.Printf("Generated Dockerfile (flavor=%s):\n%s\n", flavor, dockerfile)

	pkg := fmt.Sprintf("sandbox-%s", flavor)
	versionedTag := fmt.Sprintf("%s/%s/%s:v%s", ghcrRegistry, ghcrNamespace, pkg, bridgeVersion)
	latestTag := fmt.Sprintf("%s/%s/%s:latest", ghcrRegistry, ghcrNamespace, pkg)

	log.Printf("Building %s...", versionedTag)
	if err := dockerBuild(ctx, tmpDir, dockerfilePath, versionedTag, latestTag); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	for _, tag := range []string{versionedTag, latestTag} {
		log.Printf("Pushing %s...", tag)
		if err := dockerPush(ctx, tag); err != nil {
			return fmt.Errorf("docker push %s: %w", tag, err)
		}
		log.Printf("✓ Pushed %s", tag)
	}

	log.Printf("Verify the package is public (one-time per package): https://github.com/orgs/%s/packages/container/package/%s/settings", ghcrNamespace, pkg)

	return createDaytonaSnapshots(ctx, flavor, bridgeVersion, versionedTag, targetSizes)
}

func createDaytonaSnapshots(ctx context.Context, flavor, bridgeVersion, ghcrTag string, targetSizes []string) error {
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
		name := snapshotName(flavor, bridgeVersion, size.Name)

		log.Printf("Creating Daytona snapshot %q (cpu=%d, mem=%dGB, disk=%dGB)...",
			name, size.CPU, size.Memory, size.Disk)

		snapshot, logChan, err := client.Snapshot.Create(ctx, &types.CreateSnapshotParams{
			Name:  name,
			Image: ghcrTag,
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
		log.Printf("✓ Snapshot %q ready (id=%s)", name, snapshot.Name)
	}
	return nil
}

func dockerLogin(ctx context.Context, user, pat string) error {
	cmd := exec.CommandContext(ctx, "docker", "login", ghcrRegistry, "-u", user, "--password-stdin")
	cmd.Stdin = strings.NewReader(pat)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerBuild(ctx context.Context, contextDir, dockerfilePath string, tags ...string) error {
	args := []string{"build", "--platform", "linux/amd64", "-f", dockerfilePath}
	for _, tag := range tags {
		args = append(args, "-t", tag)
	}
	args = append(args, contextDir)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerPush(ctx context.Context, tag string) error {
	cmd := exec.CommandContext(ctx, "docker", "push", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
