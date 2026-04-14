package extractor

import (
	ruby "github.com/smacker/go-tree-sitter/ruby"
)

func init() {
	Register(LanguageSpec{
		Language:   "ruby",
		TSLanguage: ruby.GetLanguage(),
		Extensions: []string{".rb"},
		FunctionTypes: toSet([]string{
			"method",
			"singleton_method",
		}),
		ClassTypes: toSet([]string{
			"class",
			"module",
		}),
		NameField: "name",
	})
}
