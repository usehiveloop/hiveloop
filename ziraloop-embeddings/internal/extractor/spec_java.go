package extractor

import (
	java "github.com/smacker/go-tree-sitter/java"
)

func init() {
	Register(LanguageSpec{
		Language:   "java",
		TSLanguage: java.GetLanguage(),
		Extensions: []string{".java"},
		FunctionTypes: toSet([]string{
			"method_declaration",
			"constructor_declaration",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
			"interface_declaration",
			"enum_declaration",
			"record_declaration",
			"annotation_type_declaration",
		}),
		NameField: "name",
	})
}
