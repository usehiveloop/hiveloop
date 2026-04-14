package extractor

import (
	rust "github.com/smacker/go-tree-sitter/rust"
)

func init() {
	Register(LanguageSpec{
		Language:   "rust",
		TSLanguage: rust.GetLanguage(),
		Extensions: []string{".rs"},
		FunctionTypes: toSet([]string{
			"function_item",
		}),
		ClassTypes: toSet([]string{
			"struct_item",
			"enum_item",
			"trait_item",
			"impl_item",
			"type_item",
		}),
		NameField: "name",
	})
}
