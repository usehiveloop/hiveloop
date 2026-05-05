package main

import (
	"context"
	"flag"
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

func runBridge(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bridge", flag.ExitOnError)
	version := fs.String("version", "", "Image version (required, e.g. 1.0.0)")
	bridgeVersion := fs.String("bridge-version", "", "usehiveloop/bridge release tag (required, e.g. v1.0.0)")
	size := fs.String("size", "all", "Snapshot sizes to register (small, medium, large, xlarge, all)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *version == "" || *bridgeVersion == "" {
		fmt.Fprintln(os.Stderr, "error: -version and -bridge-version are required")
		os.Exit(1)
	}
	targetSizes, err := resolveSizes(*size)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := buildAndPush(ctx, *version, *bridgeVersion, targetSizes); err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Println("Done.")
}

func resolveSizes(s string) ([]string, error) {
	if s == "all" {
		return []string{"small", "medium", "large", "xlarge"}, nil
	}
	out := []string{}
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if _, ok := sizes[name]; !ok {
			return nil, fmt.Errorf("unknown size %q (valid: small, medium, large, xlarge, all)", name)
		}
		out = append(out, name)
	}
	return out, nil
}

func buildAndPush(ctx context.Context, version, bridgeVersion string, targetSizes []string) error {
	// Strip a leading "v" so downstream formatters that prepend "v" don't double it.
	version = strings.TrimPrefix(version, "v")

	user := os.Getenv("GHCR_USERNAME")
	pat := os.Getenv("GHCR_PAT")
	if user == "" || pat == "" {
		return fmt.Errorf("GHCR_USERNAME and GHCR_PAT environment variables are required")
	}

	if err := dockerLogin(ctx, user, pat); err != nil {
		return fmt.Errorf("docker login: %w", err)
	}

	dockerfile := buildBridgeImage(version, bridgeVersion).Dockerfile()

	tmpDir, err := os.MkdirTemp("", "buildtemplates-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o600); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}
	log.Printf("Generated Dockerfile:\n%s\n", dockerfile)

	pkg := "sandbox-bridge"
	versionedTag := fmt.Sprintf("%s/%s/%s:v%s", ghcrRegistry, ghcrNamespace, pkg, version)
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

	return createDaytonaSnapshots(ctx, version, versionedTag, targetSizes)
}

func createDaytonaSnapshots(ctx context.Context, version, imageRef string, targetSizes []string) error {
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
		name := snapshotName(version, size.Name)

		log.Printf("Creating Daytona snapshot %q (cpu=%d, mem=%dGB, disk=%dGB)...",
			name, size.CPU, size.Memory, size.Disk)

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

		// Create returns immediately after kicking off the build; the log stream
		// closing means the build finished but says nothing about success. Re-fetch
		// the snapshot and verify state before declaring victory.
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

