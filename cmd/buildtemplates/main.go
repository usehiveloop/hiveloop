// Command buildtemplates creates base sandbox snapshots with Bridge pre-installed.
//
// It builds templates of different sizes (small, medium, large, xlarge) and
// flavors (bridge, dev-box) on the configured sandbox provider. Currently
// supports Daytona; designed to be extended for future providers.
//
// Usage:
//
//	go run ./cmd/buildtemplates -version 0.10.0
//	go run ./cmd/buildtemplates -version 0.10.0 -size small
//	go run ./cmd/buildtemplates -version 0.10.0 -flavor dev-box
//	go run ./cmd/buildtemplates -version 0.10.0 -flavor dev-box -size medium
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
	"github.com/ziraloop/ziraloop/internal/model"
)

const (
	baseImage         = "ubuntu:24.04"
	bridgeDir         = "/usr/local/bin"
	daytonaHome       = "/home/daytona"
	bridgeReleasesURL = "https://github.com/ziraloop/bridge/releases/download"

	flavorBridge = "bridge"
	flavorDevBox = "dev-box"
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

const nvmVersion = "v0.40.4"

const goVersion = "1.24.2"

// devToolPackages are CLI tools and server binaries that ship dormant in the
// dev-box image. None of these start daemons at boot — the entrypoint only
// runs Bridge at boot. Agents start postgres/redis explicitly.
var devToolPackages = []string{
	"build-essential",
	"python3-pip",
	"python3-venv",
	"sqlite3",
	"libsqlite3-dev",
	"postgresql",
	"postgresql-client",
	"redis-server",
	"ffmpeg",
	"tmux",
	"screen",
	"zip",
	"tar",
	"gzip",
	"xz-utils",
	"zstd",
	"bzip2",
	"dnsutils",
	"net-tools",
	"httpie",
	"openssl",
	"nano",
	"libxml2-utils",
	"xmlstarlet",
	"postgresql-16-pgvector",
	"s3cmd",
}

// sizes re-exports model.TemplateSizes for local convenience.
var sizes = model.TemplateSizes

func bridgeDownloadURL(version string) string {
	return fmt.Sprintf("%s/v%s/bridge-v%s-x86_64-unknown-linux-gnu.tar.gz",
		bridgeReleasesURL, version, version)
}

// buildBaseImage installs the shared runtime layer used by every flavor:
// system packages, the Bridge binary, and CodeDB. Each flavor is responsible
// for its own GitHub tooling, additional layers, workdir, and entrypoint.
func buildBaseImage(bridgeVersion string) *daytona.DockerImage {
	downloadURL := bridgeDownloadURL(bridgeVersion)

	image := daytona.Base(baseImage)

	// System packages
	image = image.AptGet(basePackages)

	// Home directory + Bridge storage directory
	image = image.Run(fmt.Sprintf("mkdir -p %s/.bridge", daytonaHome))

	// Bridge binary
	image = image.Run(fmt.Sprintf(
		`curl -fsSL "%s" | tar -xzf - -C %s && chmod +x %s/bridge`,
		downloadURL, bridgeDir, bridgeDir,
	))

	// CodeDB binary (code intelligence for agents)
	image = image.Run(fmt.Sprintf(
		`curl -fsSL -o %s/codedb "https://github.com/justrach/codedb/releases/download/v0.2.57/codedb-linux-x86_64" && chmod +x %s/codedb`,
		bridgeDir, bridgeDir,
	))

	return image
}

// buildBridgeImage produces the default flavor: just the base runtime.
func buildBridgeImage(bridgeVersion string) *daytona.DockerImage {
	image := buildBaseImage(bridgeVersion)

	image = image.Workdir(daytonaHome)
	image = image.Entrypoint([]string{"/bin/sh", "-c", "mkdir -p /home/daytona/.bridge && /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	return image
}

// buildDevBoxImage layers a developer toolchain on top of the base bridge
// runtime: Node.js (via nvm), Chrome for Testing, and agent-browser
// for AI-driven browser automation. agent-browser's daemon starts lazily
// on the first CLI command — no pre-warming needed in the entrypoint.
func buildDevBoxImage(bridgeVersion string) *daytona.DockerImage {
	image := buildBaseImage(bridgeVersion)

	// Install nvm + Node LTS into a system-wide location and symlink the
	// resulting binaries into /usr/local/bin so non-login shells (and Bridge)
	// pick them up without needing to source nvm.sh.
	nvmInstall := strings.Join([]string{
		"set -eux",
		"export NVM_DIR=/usr/local/nvm",
		"mkdir -p $NVM_DIR",
		"curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/" + nvmVersion + "/install.sh | bash",
		". $NVM_DIR/nvm.sh",
		"nvm install --lts",
		"NODE_BIN=$(nvm which current)",
		"NODE_DIR=$(dirname $NODE_BIN)",
		"ln -sf $NODE_BIN /usr/local/bin/node",
		"ln -sf $NODE_DIR/npm /usr/local/bin/npm",
		"ln -sf $NODE_DIR/npx /usr/local/bin/npx",
	}, " && ")
	image = image.Run("bash -c '" + nvmInstall + "'")

	// Install agent-browser and other global CLIs.
	// --prefix=/usr/local forces npm to drop the bin shims into /usr/local/bin
	// instead of nvm's per-version prefix (/usr/local/nvm/versions/node/<v>/bin),
	// which is NOT on the default PATH that the Bridge entrypoint sees.
	image = image.Run("npm install -g --prefix=/usr/local agent-browser")

	// GitHub CLI (official apt repository).
	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			"echo 'deb [arch=amd64 signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main' > /etc/apt/sources.list.d/github-cli.list && " +
			"apt-get update && apt-get install -y gh")

	// Download Chrome for Testing and install its Linux shared-library
	// dependencies (libnss3, libatk, libgbm, fonts, etc.) in one step.
	// agent-browser auto-detects container environments and adds --no-sandbox.
	// HOME must be set to /home/daytona so Chrome installs into the daytona
	// user's home directory (the sandbox runs as daytona, not root).
	image = image.Run("HOME=/home/daytona agent-browser install --with-deps")

	// Dev tools: compilers, databases, media, terminal multiplexers,
	// network diagnostics, archive utilities, editors.
	// All are dormant binaries — no daemons start at boot.
	image = image.AptGet(devToolPackages)

	// yq (YAML/JSON/TOML processor) — not in apt, installed as a standalone binary.
	image = image.Run(
		`curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq`,
	)

	// Go (official tarball, symlinked into /usr/local/bin for default PATH).
	image = image.Run(fmt.Sprintf(
		"curl -fsSL https://go.dev/dl/go%s.linux-amd64.tar.gz | tar -C /usr/local -xzf - && "+
			"ln -sf /usr/local/go/bin/go /usr/local/bin/go && "+
			"ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt",
		goVersion,
	))

	// Rust via rustup (the standard version manager). RUSTUP_HOME and
	// CARGO_HOME are set to system-wide paths; key binaries are symlinked
	// into /usr/local/bin so non-login shells find them.
	image = image.Env("RUSTUP_HOME", "/usr/local/rustup")
	image = image.Env("CARGO_HOME", "/usr/local/cargo")
	image = image.Run(
		"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path && " +
			"ln -sf /usr/local/cargo/bin/rustc /usr/local/bin/rustc && " +
			"ln -sf /usr/local/cargo/bin/cargo /usr/local/bin/cargo && " +
			"ln -sf /usr/local/cargo/bin/rustup /usr/local/bin/rustup",
	)

	// Code intelligence: structural graph, AST parsing, vector search.
	// These are the building blocks for Greptile-level code review:
	//   - codebase-memory-mcp: builds a code graph (calls, refs, deps) from
	//     tree-sitter ASTs, queryable via MCP tools. Single static binary.
	//   - tree-sitter CLI: AST parsing for function-granularity chunking
	//     (agents generate docstrings per function, embed them in pgvector).
	//   - pgvector: added via apt above (postgresql-16-pgvector).
	image = image.Run(
		`curl -fsSL https://raw.githubusercontent.com/DeusData/codebase-memory-mcp/main/install.sh | bash -s -- --dir=/usr/local/bin --skip-config`,
	)
	image = image.Run(
		"curl -fsSL https://github.com/tree-sitter/tree-sitter/releases/latest/download/tree-sitter-linux-x64.gz | gunzip > /usr/local/bin/tree-sitter && " +
			"chmod +x /usr/local/bin/tree-sitter",
	)

	// Git credential helper — fetches GitHub tokens from the control plane
	// on demand. Token never touches disk; git calls this on every auth request.
	image = image.Run(
		`printf '#!/bin/sh\ncurl -sf -X POST -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" "$ZIRALOOP_GIT_CREDENTIALS_URL"\n' > /usr/local/bin/git-credential-ziraloop && ` +
			`chmod +x /usr/local/bin/git-credential-ziraloop`,
	)
	image = image.Run("git config --system credential.helper /usr/local/bin/git-credential-ziraloop")

	image = image.Workdir(daytonaHome)
	// agent-browser's daemon starts lazily on the first CLI command, so
	// the entrypoint only needs to launch Bridge. No daemon pre-warming
	// or health check is required.
	image = image.Entrypoint([]string{"/bin/sh", "-c",
		"mkdir -p /home/daytona/.bridge && " +
			"exec /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	return image
}

// snapshotName returns the published snapshot name for a flavor + version + size.
// The dev-box flavor uses the new zira-dev-box-<size>-v<version> scheme; the
// default bridge flavor keeps its historical name so existing snapshots aren't
// orphaned.
func snapshotName(flavor, bridgeVersion, size string) string {
	switch flavor {
	case flavorDevBox:
		return fmt.Sprintf("zira-dev-box-%s-v%s", size, bridgeVersion)
	default:
		return fmt.Sprintf("ziraloop-bridge-%s-%s", strings.ReplaceAll(bridgeVersion, ".", "-"), size)
	}
}

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
	flavor := flag.String("flavor", flavorBridge, "Image flavor to build (bridge, dev-box)")
	size := flag.String("size", "all", "Template size to build (small, medium, large, xlarge, all)")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required")
		flag.Usage()
		os.Exit(1)
	}

	switch *flavor {
	case flavorBridge, flavorDevBox:
	default:
		fmt.Fprintf(os.Stderr, "error: unknown flavor %q (valid: %s, %s)\n", *flavor, flavorBridge, flavorDevBox)
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
		err = buildDaytona(ctx, *flavor, *version, targetSizes)
	default:
		err = fmt.Errorf("unsupported provider: %s", *provider)
	}

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Println("All templates built successfully.")
}
