package extractor

import (
	python "github.com/smacker/go-tree-sitter/python"
)

func init() {
	Register(LanguageSpec{
		Language:   "python",
		TSLanguage: python.GetLanguage(),
		Extensions: []string{".py"},
		FunctionTypes: toSet([]string{
			"function_definition",
		}),
		ClassTypes: toSet([]string{
			"class_definition",
		}),
		NameField: "name",
	})
}
