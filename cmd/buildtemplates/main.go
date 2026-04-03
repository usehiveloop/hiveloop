// Command buildtemplates creates base sandbox snapshots with Bridge pre-installed.
//
// It builds 4 templates of different sizes (small, medium, large, xlarge) on the
// configured sandbox provider. Currently supports Daytona; designed to be extended
// for future providers.
//
// Usage:
//
//	go run ./cmd/buildtemplates -version 0.10.0
//	go run ./cmd/buildtemplates -version 0.10.0 -size small
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

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"
	"github.com/daytonaio/daytona/libs/sdk-go/pkg/types"
)

const (
	baseImage         = "ubuntu:24.04"
	bridgeDir         = "/usr/local/bin"
	daytonaHome       = "/home/daytona"
	bridgeReleasesURL = "https://github.com/useportal-app/bridge/releases/download"
)

var basePackages = []string{
	"curl",
	"ca-certificates",
	"git",
	"jq",
	"unzip",
	"wget",
	"openssh-client",
}

// templateSize defines a sandbox template variant.
type templateSize struct {
	Name   string
	CPU    int
	Memory int // GB
	Disk   int // GB
}

var sizes = map[string]templateSize{
	"small":  {Name: "small", CPU: 1, Memory: 2, Disk: 10},
	"medium": {Name: "medium", CPU: 2, Memory: 4, Disk: 20},
	"large":  {Name: "large", CPU: 4, Memory: 8, Disk: 40},
	"xlarge": {Name: "xlarge", CPU: 8, Memory: 16, Disk: 80},
}

func bridgeDownloadURL(version string) string {
	return fmt.Sprintf("%s/v%s/bridge-v%s-x86_64-unknown-linux-gnu.tar.gz",
		bridgeReleasesURL, version, version)
}

// buildBridgeImage creates a DockerImage with Bridge pre-installed.
func buildBridgeImage(bridgeVersion string) *daytona.DockerImage {
	downloadURL := bridgeDownloadURL(bridgeVersion)

	image := daytona.Base(baseImage)

	// System packages
	image = image.AptGet(basePackages)

	// GitHub CLI
	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	// Home directory
	image = image.Run(fmt.Sprintf("mkdir -p %s", daytonaHome))

	// Bridge binary
	image = image.Run(fmt.Sprintf(
		`curl -fsSL "%s" | tar -xzf - -C %s && chmod +x %s/bridge`,
		downloadURL, bridgeDir, bridgeDir,
	))

	// CodeDB binary (code intelligence for agents)
	image = image.Run(fmt.Sprintf(
		`curl -fsSL "https://github.com/justrach/codedb/releases/latest/download/codedb-x86_64-linux" -o %s/codedb && chmod +x %s/codedb`,
		bridgeDir, bridgeDir,
	))

	// Working directory and entrypoint — start Bridge automatically
	image = image.Workdir(daytonaHome)
	image = image.Entrypoint([]string{"/bin/sh", "-c", "/usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	return image
}

func snapshotName(bridgeVersion, size string) string {
	return fmt.Sprintf("llmvault-bridge-%s-%s", strings.ReplaceAll(bridgeVersion, ".", "-"), size)
}

func buildDaytona(ctx context.Context, bridgeVersion string, targetSizes []string) error {
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: os.Getenv("SANDBOX_PROVIDER_KEY"),
		APIUrl: os.Getenv("SANDBOX_PROVIDER_URL"),
		Target: os.Getenv("SANDBOX_TARGET"),
	})
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	image := buildBridgeImage(bridgeVersion)
	log.Printf("Generated Dockerfile:\n%s\n", image.Dockerfile())

	for _, sizeName := range targetSizes {
		size, ok := sizes[sizeName]
		if !ok {
			return fmt.Errorf("unknown size: %s", sizeName)
		}

		name := snapshotName(bridgeVersion, size.Name)
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

		// Stream build logs
		for line := range logChan {
			log.Printf("[%s] %s", name, line)
		}

		log.Printf("Snapshot %q created successfully (id=%s)", name, snapshot.Name)
	}

	return nil
}

func main() {
	version := flag.String("version", "", "Bridge version to install (required, e.g. 0.10.0)")
	provider := flag.String("provider", "daytona", "Sandbox provider (daytona)")
	size := flag.String("size", "all", "Template size to build (small, medium, large, xlarge, all)")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required")
		flag.Usage()
		os.Exit(1)
	}

	// Determine which sizes to build
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
		err = buildDaytona(ctx, *version, targetSizes)
	default:
		err = fmt.Errorf("unsupported provider: %s", *provider)
	}

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Println("All templates built successfully.")
}
