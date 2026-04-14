package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// LanguageSpec defines how to extract symbols from a specific language's AST.
type LanguageSpec struct {
	Language      string
	TSLanguage    *sitter.Language
	Extensions    []string
	FunctionTypes map[string]bool // node types for functions/methods
	ClassTypes    map[string]bool // node types for classes/structs/interfaces/enums
	NameField     string          // tree-sitter field name for symbol name (usually "name")
	// NameExtractor is an optional fallback for languages where the name field
	// varies by node type (e.g. JS arrow functions assigned to variables).
	NameExtractor func(node *sitter.Node, source []byte) string
}

var (
	specsByExt  = map[string]*LanguageSpec{}
	specsByLang = map[string]*LanguageSpec{}
	allExts     []string
)

// Register adds a language spec to the registry.
func Register(spec LanguageSpec) {
	specsByLang[spec.Language] = &spec
	for _, ext := range spec.Extensions {
		specsByExt[ext] = &spec
		allExts = append(allExts, ext)
	}
}

// ForExtension returns the language spec for a file extension, or nil.
func ForExtension(ext string) *LanguageSpec {
	return specsByExt[ext]
}

// SupportedExtensions returns all registered file extensions.
func SupportedExtensions() []string {
	return allExts
}

func toSet(items []string) map[string]bool {
	result := make(map[string]bool, len(items))
	for _, item := range items {
		result[item] = true
	}
	return result
}
