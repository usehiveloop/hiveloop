package extractor

import (
	"os"
	"path/filepath"
	"strings"
)

// Directories to skip during file scanning.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, ".next": true, "__pycache__": true, ".ignored": true,
	"zig-cache": true, "zig-out": true, ".turbo": true, ".vercel": true,
	"coverage": true, ".nyc_output": true, "target": true, ".idea": true,
	".vscode": true, ".cache": true, "tmp": true, ".tmp": true,
}

// RepoName derives a short name from a repo path.
func RepoName(repoPath string) string {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}
	return filepath.Base(absPath)
}

// CollectFiles walks a repo and returns source files grouped by language.
func CollectFiles(repoPath string) ([]string, map[string]int, error) {
	var files []string
	langCounts := make(map[string]int)

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		spec := ForExtension(ext)
		if spec == nil {
			return nil
		}

		files = append(files, path)
		langCounts[spec.Language]++
		return nil
	})

	return files, langCounts, err
}

// DetectLanguage returns the language spec for a file, checking extension
// and falling back to content-based detection for ambiguous cases.
func DetectLanguage(filePath string, content []byte) *LanguageSpec {
	ext := filepath.Ext(filePath)

	// Handle ambiguous extensions
	if ext == ".h" {
		return detectHeaderLanguage(content)
	}

	return ForExtension(ext)
}

func detectHeaderLanguage(content []byte) *LanguageSpec {
	contentStr := string(content)
	// Check for C++ indicators
	cppIndicators := []string{"#include <iostream>", "#include <string>", "#include <vector>",
		"namespace ", "class ", "template<", "template <", "std::"}
	for _, indicator := range cppIndicators {
		if strings.Contains(contentStr, indicator) {
			return specsByLang["cpp"]
		}
	}
	// Default .h to C
	return specsByLang["c"]
}
