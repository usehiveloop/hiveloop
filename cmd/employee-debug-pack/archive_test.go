package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestExtractTarGzExtractsRegularFiles(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "debug.tar.gz")
	writeTestArchive(t, archivePath, map[string]string{
		"debug/SUMMARY.txt":        "summary",
		"debug/system/uname.txt":   "uname",
		"debug/env/env.redacted":   "TOKEN=<redacted>",
		"debug/logs/empty-folder/": "",
	})

	extracted, err := extractTarGz(archivePath, tmp)
	if err != nil {
		t.Fatalf("extractTarGz returned error: %v", err)
	}
	sort.Strings(extracted)

	want := []string{
		filepath.Join(tmp, "debug/SUMMARY.txt"),
		filepath.Join(tmp, "debug/env/env.redacted"),
		filepath.Join(tmp, "debug/system/uname.txt"),
	}
	if !reflect.DeepEqual(extracted, want) {
		t.Fatalf("extracted files mismatch\nwant: %#v\n got: %#v", want, extracted)
	}
	assertFileContent(t, filepath.Join(tmp, "debug/SUMMARY.txt"), "summary")
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "debug.tar.gz")
	writeTestArchive(t, archivePath, map[string]string{
		"../escape.txt": "bad",
	})

	if _, err := extractTarGz(archivePath, tmp); err == nil {
		t.Fatal("extractTarGz returned nil error for path traversal entry")
	}
}

func TestLoadEnvFileDoesNotOverrideExistingValues(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	content := "SANDBOX_PROVIDER_KEY=from-file\nSANDBOX_PROVIDER_URL='https://example.test'\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("SANDBOX_PROVIDER_KEY", "from-env")
	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("loadEnvFile returned error: %v", err)
	}

	if got := os.Getenv("SANDBOX_PROVIDER_KEY"); got != "from-env" {
		t.Fatalf("SANDBOX_PROVIDER_KEY overridden: %q", got)
	}
	if got := os.Getenv("SANDBOX_PROVIDER_URL"); got != "https://example.test" {
		t.Fatalf("SANDBOX_PROVIDER_URL = %q", got)
	}
}

func writeTestArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for name, content := range files {
		if name[len(name)-1] == '/' {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
				t.Fatalf("write dir header: %v", err)
			}
			continue
		}
		header := &tar.Header{
			Name:     name,
			Mode:     0o600,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write file header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write file body: %v", err)
		}
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}
