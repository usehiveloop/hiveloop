package main

import (
	"fmt"
	"strings"

	daytona "github.com/daytonaio/daytona/libs/sdk-go/pkg/daytona"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	baseImage         = "ubuntu:24.04"
	bridgeDir         = "/usr/local/bin"
	daytonaHome       = "/home/daytona"
	bridgeReleasesURL = "https://github.com/usehiveloop/bridge/releases/download"

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

const astGrepVersion = "0.42.1"

// devToolPackages are CLI tools and server binaries that ship dormant in the
// dev-box image. None of these start daemons at boot.
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
	"s3cmd",
	"ripgrep",
}

var sizes = model.TemplateSizes

// snapshotName returns the Daytona snapshot name for (flavor, version, size).
// The naming scheme matches what was published before the GHCR migration so
// existing references keep working.
func snapshotName(flavor, bridgeVersion, size string) string {
	switch flavor {
	case flavorDevBox:
		return fmt.Sprintf("hiveloop-dev-box-%s-v%s", size, bridgeVersion)
	default:
		return fmt.Sprintf("hiveloop-bridge-%s-%s", strings.ReplaceAll(bridgeVersion, ".", "-"), size)
	}
}

func bridgeDownloadURL(version string) string {
	return fmt.Sprintf("%s/v%s/bridge-v%s-x86_64-unknown-linux-gnu.tar.gz",
		bridgeReleasesURL, version, version)
}

func buildBaseImage(bridgeVersion string) *daytona.DockerImage {
	downloadURL := bridgeDownloadURL(bridgeVersion)

	image := daytona.Base(baseImage)

	image = image.AptGet(basePackages)
	image = image.Run(fmt.Sprintf("mkdir -p %s/.bridge", daytonaHome))
	image = image.Run(fmt.Sprintf(
		`curl -fsSL "%s" | tar -xzf - -C %s && chmod +x %s/bridge`,
		downloadURL, bridgeDir, bridgeDir,
	))

	return image
}

func buildBridgeImage(bridgeVersion string) *daytona.DockerImage {
	image := buildBaseImage(bridgeVersion)

	image = image.Workdir(daytonaHome)
	image = image.Entrypoint([]string{"/bin/sh", "-c", "mkdir -p /home/daytona/.bridge && /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	return image
}

func buildDevBoxImage(bridgeVersion string) *daytona.DockerImage {
	image := buildBaseImage(bridgeVersion)

	// Install nvm + Node LTS into a system-wide location and symlink the
	// resulting binaries into /usr/local/bin so non-login shells find them.
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

	image = image.Run("npm install -g --prefix=/usr/local agent-browser")

	image = image.Run(
		"mkdir -p -m 755 /etc/apt/keyrings && " +
			"wget -qO- https://cli.github.com/packages/githubcli-archive-keyring.gpg | tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null && " +
			"chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg && " +
			"echo 'deb [arch=amd64 signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main' > /etc/apt/sources.list.d/github-cli.list && " +
			"apt-get update && apt-get install -y gh && rm -rf /var/lib/apt/lists/*")

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

	image = image.Run("/usr/local/bin/bridge install-lsp all")

	// Git credential helper — fetches GitHub tokens from the control plane on demand.
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

	image = image.Workdir(daytonaHome)
	image = image.Entrypoint([]string{"/bin/sh", "-c",
		"mkdir -p /home/daytona/.bridge && " +
			"exec /usr/local/bin/bridge >> /tmp/bridge.log 2>&1"})

	return image
}

