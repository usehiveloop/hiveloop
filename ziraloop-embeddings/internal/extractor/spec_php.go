package extractor

import (
	php "github.com/smacker/go-tree-sitter/php"
)

func init() {
	Register(LanguageSpec{
		Language:   "php",
		TSLanguage: php.GetLanguage(),
		Extensions: []string{".php"},
		FunctionTypes: toSet([]string{
			"function_definition",
			"method_declaration",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
			"interface_declaration",
			"trait_declaration",
			"enum_declaration",
		}),
		NameField: "name",
	})
}
