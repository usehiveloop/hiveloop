package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"
	javascript "github.com/smacker/go-tree-sitter/javascript"
)

func init() {
	Register(LanguageSpec{
		Language:   "javascript",
		TSLanguage: javascript.GetLanguage(),
		Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
		FunctionTypes: toSet([]string{
			"function_declaration",
			"method_definition",
			"arrow_function",
			"generator_function_declaration",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
		}),
		NameField: "name",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			return tsNameExtractor(node, source) // reuse TS logic
		},
	})
}
