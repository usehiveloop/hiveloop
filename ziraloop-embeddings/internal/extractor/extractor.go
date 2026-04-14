package extractor

import (
	"os"
	"path/filepath"

	"github.com/ziraloop/ziraloop-embeddings/internal/model"
)

// ScanAndExtract walks the repo, extracts symbols from all supported files.
// Returns symbols, language file counts, and any error.
func ScanAndExtract(repoPath string) ([]model.Symbol, map[string]int, error) {
	files, langCounts, err := CollectFiles(repoPath)
	if err != nil {
		return nil, nil, err
	}

	var allSymbols []model.Symbol
	for _, filePath := range files {
		source, readErr := os.ReadFile(filePath)
		if readErr != nil {
			continue
		}

		ext := filepath.Ext(filePath)
		spec := ForExtension(ext)
		if spec == nil {
			continue
		}

		symbols := ExtractSymbols(filePath, source, spec, repoPath)
		allSymbols = append(allSymbols, symbols...)
	}

	return allSymbols, langCounts, nil
}

// ExtractFiles extracts symbols from a specific list of files (relative paths).
// Used for incremental re-indexing of changed files.
func ExtractFiles(repoPath string, relPaths []string) ([]model.Symbol, error) {
	var allSymbols []model.Symbol

	for _, relPath := range relPaths {
		absPath := filepath.Join(repoPath, relPath)
		source, err := os.ReadFile(absPath)
		if err != nil {
			continue // file might have been deleted
		}

		ext := filepath.Ext(absPath)
		spec := ForExtension(ext)
		if spec == nil {
			continue
		}

		symbols := ExtractSymbols(absPath, source, spec, repoPath)
		allSymbols = append(allSymbols, symbols...)
	}

	return allSymbols, nil
}
