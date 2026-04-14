package extractor

import (
	csharp "github.com/smacker/go-tree-sitter/csharp"
)

func init() {
	Register(LanguageSpec{
		Language:   "csharp",
		TSLanguage: csharp.GetLanguage(),
		Extensions: []string{".cs"},
		FunctionTypes: toSet([]string{
			"method_declaration",
			"constructor_declaration",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
			"interface_declaration",
			"struct_declaration",
			"enum_declaration",
			"record_declaration",
		}),
		NameField: "name",
	})
}
