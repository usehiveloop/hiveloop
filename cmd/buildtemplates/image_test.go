package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildBridgeImageDownloadsBridgeFromMonorepoRelease(t *testing.T) {
	dockerfile := buildBridgeImage("1.2.3", "v1.2.3", false).Dockerfile()

	want := "https://github.com/usehiveloop/hiveloop/releases/download/v1.2.3/bridge-v1.2.3-x86_64-unknown-linux-gnu.tar.gz"
	if !strings.Contains(dockerfile, want) {
		t.Fatalf("Dockerfile missing monorepo bridge asset URL %q:\n%s", want, dockerfile)
	}
	if strings.Contains(dockerfile, "github.com/usehiveloop/bridge/releases") {
		t.Fatalf("Dockerfile still references the old bridge release repo:\n%s", dockerfile)
	}
	if strings.Contains(dockerfile, "COPY bridge /usr/local/bin/bridge") {
		t.Fatalf("remote-release Dockerfile unexpectedly copies local bridge binary:\n%s", dockerfile)
	}
}

func TestBuildBridgeImageCanUseLocalBridgeBinary(t *testing.T) {
	dockerfile := buildBridgeImage("1.2.3", "v1.2.3", true).Dockerfile()

	if !strings.Contains(dockerfile, "COPY bridge /usr/local/bin/bridge") {
		t.Fatalf("Dockerfile missing local bridge binary copy:\n%s", dockerfile)
	}
	if strings.Contains(dockerfile, "bridge-v1.2.3-x86_64-unknown-linux-gnu.tar.gz") {
		t.Fatalf("local-binary Dockerfile unexpectedly downloads release asset:\n%s", dockerfile)
	}
}

func TestValidateLinuxAMD64ELFRejectsNonELF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge")
	if err := os.WriteFile(path, []byte("not an elf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateLinuxAMD64ELF(path); err == nil {
		t.Fatalf("expected non-ELF bridge binary to be rejected")
	}
}
