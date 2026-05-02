package main

import (
	"fmt"
	"strings"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	// node:22-bookworm-slim matches the runtime base in useportal.bridge's
	// own Dockerfile, so the bridge binary's expectations (HOME=/work,
	// claude-agent-acp + opencode-ai installed globally) carry over directly.
	baseImage = "node:22-bookworm-slim"
	workDir   = "/work"

	// bridgeDownloadURL is the placeholder download URL for the new ACP-harness
	// bridge binary. Once useportal.bridge ships GitHub releases, replace this
	// with the real release-asset URL (and ideally fetch a checksum alongside).
	// Keep this in sync with internal/sandbox/daytona/driver_snapshot.go.
	//
	// TODO(migration): replace with the real release URL for useportal.bridge.
	bridgeDownloadURL = "https://github.com/useportal/bridge/releases/download/TODO-MIGRATION-rip-harness/bridge-linux-x86_64"

	// ACP harness package versions — must match useportal.bridge's
	// docker/Dockerfile so bridge dispatches to a known-compatible binary.
	claudeACPVersion = "0.31.4"
	openCodeVersion  = "1.14.32"
)

// basePackages are the apt packages installed before any tooling. tini is the
// init shim bridge uses as PID 1; the rest are tools agents call directly
// (curl/wget/git/jq/unzip/openssh-client/gh).
var basePackages = []string{
	"ca-certificates",
	"curl",
	"git",
	"jq",
	"openssh-client",
	"tini",
	"unzip",
	"wget",
}

const nvmVersion = "v0.40.4"

const goVersion = "1.24.2"

const astGrepVersion = "0.42.1"

// devToolPackages are dormant CLI / runtime packages agents use during their
// work. None of these start daemons at boot — the entrypoint is bridge.
var devToolPackages = []string{
	"build-essential",
	"python3-pip",
	"python3-venv",
	"sqlite3",
	"libsqlite3-dev",
	"postgresql-client",
	"redis-tools",
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
	"s3cmd",
	"ripgrep",
}

var sizes = model.TemplateSizes

// snapshotName matches the BridgeBaseImagePrefix default in
// internal/config/config.go (`hiveloop-bridge-1-0-0-small-v1`). Version dots
// become dashes; the trailing `-v1` is the runtime-contract revision and
// bumps when the image's startup contract changes (not when the bridge
// binary version bumps).
func snapshotName(version, size string) string {
	return fmt.Sprintf("hiveloop-bridge-%s-%s-v1", strings.ReplaceAll(version, ".", "-"), size)
}

func buildBridgeImage() *daytona.DockerImage {
	image := daytona.Base(baseImage)

	image = image.AptGet(basePackages)

	// gh CLI from GitHub's official apt repo.
	image = image.Run(
		"curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && " +
			`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && ` +
			"apt-get update && apt-get install -y --no-install-recommends gh && rm -rf /var/lib/apt/lists/*",
	)

	// ACP harnesses installed globally so bridge can spawn them as subprocesses
	// based on AgentDefinition.harness.
	image = image.Run(fmt.Sprintf(
		"npm install -g @agentclientprotocol/claude-agent-acp@%s opencode-ai@%s && npm cache clean --force",
		claudeACPVersion, openCodeVersion,
	))

	// nvm + extra Node LTS for agents that need to switch versions during a
	// project. The system node from the base image stays as `/usr/local/bin/node`.
	nvmInstall := strings.Join([]string{
		"set -eux",
		"export NVM_DIR=/usr/local/nvm",
		"mkdir -p $NVM_DIR",
		"curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/" + nvmVersion + "/install.sh | bash",
		". $NVM_DIR/nvm.sh",
		"nvm install --lts",
	}, " && ")
	image = image.Run("bash -c '" + nvmInstall + "'")

	image = image.Run("npm install -g --prefix=/usr/local agent-browser")
	image = image.Run("agent-browser install --with-deps")

	image = image.AptGet(devToolPackages)

	image = image.Run(
		`curl -fsSL https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq`,
	)

	image = image.Run(fmt.Sprintf(
		`curl -fsSL https://github.com/ast-grep/ast-grep/releases/download/%s/app-x86_64-unknown-linux-gnu.zip -o /tmp/ast-grep.zip && `+
			`unzip -o /tmp/ast-grep.zip -d /usr/local/bin/ && `+
			`chmod +x /usr/local/bin/ast-grep /usr/local/bin/sg && `+
			`rm /tmp/ast-grep.zip`,
		astGrepVersion,
	))

	image = image.Run(fmt.Sprintf(
		"curl -fsSL https://go.dev/dl/go%s.linux-amd64.tar.gz | tar -C /usr/local -xzf - && "+
			"ln -sf /usr/local/go/bin/go /usr/local/bin/go && "+
			"ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt",
		goVersion,
	))

	image = image.Env("RUSTUP_HOME", "/usr/local/rustup")
	image = image.Env("CARGO_HOME", "/usr/local/cargo")
	image = image.Run(
		"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path && " +
			"ln -sf /usr/local/cargo/bin/rustc /usr/local/bin/rustc && " +
			"ln -sf /usr/local/cargo/bin/cargo /usr/local/bin/cargo && " +
			"ln -sf /usr/local/cargo/bin/rustup /usr/local/bin/rustup",
	)

	// rtk: bash output token-trimmer that bridge prepends to ~85 routable commands
	// (git, npm, composer, pytest, cargo, etc.). Without it bridge logs a warning
	// and runs commands unfiltered, burning many more LLM tokens per call.
	image = image.Run("curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh")

	// uv/uvx: standard launcher for Python-based stdio MCP servers (e.g. `uvx <pkg>`).
	// Installer drops into $HOME/.local/bin; symlink so non-login shells find it.
	image = image.Run(
		"curl -LsSf https://astral.sh/uv/install.sh | sh && " +
			"ln -sf /root/.local/bin/uv /usr/local/bin/uv && " +
			"ln -sf /root/.local/bin/uvx /usr/local/bin/uvx",
	)

	// Git credential helper — fetches GitHub tokens from the control plane on demand.
	// Auth uses BRIDGE_CONTROL_PLANE_API_KEY (the bridge ↔ control-plane shared
	// secret the orchestrator sets in baseEnvVars); the legacy
	// HIVELOOP_SANDBOX_TOKEN was sandbox-agent's own auth.
	image = image.Run(
		`printf '#!/bin/sh\ncurl -sf -X POST -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" "$HIVELOOP_GIT_CREDENTIALS_URL"\n' > /usr/local/bin/git-credential-hiveloop && ` +
			`chmod +x /usr/local/bin/git-credential-hiveloop`,
	)
	image = image.Run("git config --system credential.helper /usr/local/bin/git-credential-hiveloop")
	image = image.Run("git config --system user.name hiveloop")
	image = image.Run("git config --system user.email help@hiveloop.com")
	image = image.Run("git config --global user.name hiveloop")
	image = image.Run("git config --global user.email help@hiveloop.com")

	// gh CLI wrapper — fetches a fresh GitHub token on every invocation.
	image = image.Run(
		`printf '#!/bin/sh\nexport GH_NO_KEYRING=1\nexport GH_TOKEN=$(curl -sf -X POST -H "Authorization: Bearer $BRIDGE_CONTROL_PLANE_API_KEY" "$HIVELOOP_GIT_CREDENTIALS_URL" | grep password | cut -d= -f2)\nexec /usr/bin/gh "$@"\n' > /usr/local/bin/gh-wrapper && ` +
			`chmod +x /usr/local/bin/gh-wrapper && ` +
			`ln -sf /usr/local/bin/gh-wrapper /usr/local/bin/gh`,
	)

	// Bridge binary itself.
	image = image.Run(fmt.Sprintf(
		`curl -fsSL %q -o /usr/local/bin/bridge && chmod +x /usr/local/bin/bridge`,
		bridgeDownloadURL,
	))

	// Per-harness state lives under HOME=/work so the bridge SQLite DB and
	// each ACP harness's session backups survive provider stop/start. Mirror
	// the env to image-level so a manual `docker run` (without the
	// orchestrator) lands in the same shape.
	image = image.Run("mkdir -p /work/.claude /work/.opencode /work/tmp")
	image = image.Env("HOME", workDir)
	image = image.Env("CLAUDE_CONFIG_DIR", "/work/.claude")
	image = image.Env("OPENCODE_CONFIG_DIR", "/work/.opencode")
	image = image.Env("TMPDIR", "/work/tmp")
	image = image.Env("NO_BROWSER", "1")

	image = image.Workdir(workDir)
	image = image.Entrypoint([]string{
		"/usr/bin/tini", "--",
		"/bin/sh", "-c",
		"mkdir -p /work/.claude /work/.opencode /work/tmp && exec /usr/local/bin/bridge >> /tmp/bridge.log 2>&1",
	})

	return image
}
