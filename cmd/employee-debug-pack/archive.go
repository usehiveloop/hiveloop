package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func extractTarGz(archivePath string, localDir string) ([]string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	gz, err := gzip.NewReader(archive)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	localDir, err = filepath.Abs(localDir)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	var files []string
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := path.Clean(header.Name)
		if name == "." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return nil, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		target := filepath.Join(localDir, filepath.FromSlash(name))
		if !isWithinDir(localDir, target) {
			return nil, fmt.Errorf("archive path escapes destination: %q", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeTarFile(target, tr); err != nil {
				return nil, err
			}
			files = append(files, target)
		}
	}
	return files, nil
}

func writeTarFile(target string, tr *tar.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, tr); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func isWithinDir(parent string, target string) bool {
	parent = filepath.Clean(parent)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(parent, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
