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

// chromeRuntimePackages are the shared libraries chrome-headless-shell needs
// to start on Ubuntu 24.04 (noble). Several library packages got the t64
// suffix in noble due to the time_t transition.
var chromeRuntimePackages = []string{
	"libnss3",
	"libatk-bridge2.0-0t64",
	"libatk1.0-0t64",
	"libcups2t64",
	"libdrm2",
	"libgbm1",
	"libxkbcommon0",
	"libxcomposite1",
	"libxdamage1",
	"libxfixes3",
	"libxrandr2",
	"libasound2t64",
	"libpango-1.0-0",
	"libcairo2",
	"fonts-liberation",
	"fonts-noto-color-emoji",
}

const nvmVersion = "v0.40.4"

const goVersion = "1.24.2"

// devToolPackages are CLI tools and server binaries that ship dormant in the
// dev-box image. None of these start daemons at boot — the entrypoint only
// runs chrome-devtools-axi and Bridge. Agents start postgres/redis explicitly.
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
		`curl -fsSL -o %s/codedb "https://github.com/justrach/codedb/releases/download/v0.2.54/codedb-linux-x86_64" && chmod +x %s/codedb`,
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
// runtime: Node.js (via nvm), chrome-headless-shell, and chrome-devtools-axi
// for AI-driven browser automation. The chrome-devtools-axi daemon is
// pre-warmed by the entrypoint so the first browser command pays no cold start.
func buildDevBoxImage(bridgeVersion string) *daytona.DockerImage {
	image := buildBaseImage(bridgeVersion)

	// Chromium runtime libs (chrome-headless-shell needs these to start).
	image = image.AptGet(chromeRuntimePackages)

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

	// Bridge is the runtime — not Claude Code or Codex — so the axi-sdk-js
	// session-start hooks that write to ~/.claude/ and ~/.codex/ are dead weight.
	image = image.Env("CHROME_DEVTOOLS_AXI_DISABLE_HOOKS", "1")

	// chrome-devtools-mcp (the backend chrome-devtools-axi delegates to) uses
	// Puppeteer with channel: 'stable', which looks for /opt/google/chrome/chrome.
	// We install chrome-headless-shell (lean, ~170 MB) and symlink it there.
	image = image.Run(
		"npx -y @puppeteer/browsers install chrome-headless-shell@stable --path /home/daytona/.cache/puppeteer && " +
			"mkdir -p /opt/google/chrome && " +
			"ln -sf $(find /home/daytona/.cache/puppeteer -type f -name chrome-headless-shell | head -n1) /opt/google/chrome/chrome",
	)

	// Chrome needs --no-sandbox when running as root, --disable-dev-shm-usage
	// for small /dev/shm, and --disable-gpu in headless containers. Docker ENV
	// can't hold spaces in values (the SDK doesn't quote them), so we set the
	// env var via profile.d (interactive SSH) and the entrypoint (daemon).
	image = image.Run(
		`echo 'export CHROME_DEVTOOLS_AXI_CHROME_ARGS="--no-sandbox --disable-dev-shm-usage --disable-gpu"' > /etc/profile.d/chrome-args.sh`,
	)

	// Install the AXI CLIs globally. dev-box uses gh-axi instead of the
	// standard gh CLI: it sits on the same axi-sdk-js runtime, ships TOON
	// output, and outperforms gh on the GitHub benchmark across cost,
	// duration, and turn count.
	//
	// --prefix=/usr/local forces npm to drop the bin shims into /usr/local/bin
	// instead of nvm's per-version prefix (/usr/local/nvm/versions/node/<v>/bin),
	// which is NOT on the default PATH that the Bridge entrypoint sees. Without
	// this, `chrome-devtools-axi: not found` at sandbox start.
	image = image.Run("npm install -g --prefix=/usr/local chrome-devtools-axi gh-axi vercel")

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

	// Railway CLI — infrastructure deployment and management.
	image = image.Run("bash <(curl -fsSL cli.new)")

	// Render CLI — manage Render services from the command line.
	image = image.Run("curl -fsSL https://raw.githubusercontent.com/render-oss/cli/refs/heads/main/bin/install.sh | sh")

	image = image.Workdir(daytonaHome)
	// Fail-fast entrypoint: if chrome-devtools-axi can't start its daemon,
	// Bridge does not launch. This makes Bridge /health on :8080 a transitive
	// proof that the daemon is also healthy — the only signal we can reach
	// from outside the sandbox, since the daemon binds 127.0.0.1:9224.
	//
	// The curl --retry loop blocks until the daemon is actually accepting
	// connections (chrome-devtools-axi start forks and returns before listen
	// is bound), eliminating the race where Bridge could come up before the
	// daemon is ready to serve.
	image = image.Entrypoint([]string{"/bin/sh", "-c",
		"export CHROME_DEVTOOLS_AXI_CHROME_ARGS='--no-sandbox --disable-dev-shm-usage --disable-gpu' && " +
			"mkdir -p /home/daytona/.bridge && " +
			"chrome-devtools-axi start >> /tmp/cda.log 2>&1 && " +
			"curl --retry 30 --retry-delay 1 --retry-connrefused -sf http://127.0.0.1:9224/health >> /tmp/cda.log 2>&1 && " +
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
