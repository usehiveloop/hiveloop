package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"
	tsx "github.com/smacker/go-tree-sitter/typescript/tsx"
)

func init() {
	Register(LanguageSpec{
		Language:   "typescript",
		TSLanguage: typescript.GetLanguage(),
		Extensions: []string{".ts"},
		FunctionTypes: toSet([]string{
			"function_declaration",
			"method_definition",
			"arrow_function",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
			"interface_declaration",
			"type_alias_declaration",
			"enum_declaration",
		}),
		NameField: "name",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			return tsNameExtractor(node, source)
		},
	})

	Register(LanguageSpec{
		Language:   "tsx",
		TSLanguage: tsx.GetLanguage(),
		Extensions: []string{".tsx"},
		FunctionTypes: toSet([]string{
			"function_declaration",
			"method_definition",
			"arrow_function",
		}),
		ClassTypes: toSet([]string{
			"class_declaration",
			"interface_declaration",
			"type_alias_declaration",
			"enum_declaration",
		}),
		NameField: "name",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			return tsNameExtractor(node, source)
		},
	})
}

// tsNameExtractor handles arrow functions assigned to variables:
// const Foo = () => { ... }
func tsNameExtractor(node *sitter.Node, source []byte) string {
	if node.Type() == "arrow_function" {
		parent := node.Parent()
		if parent != nil && parent.Type() == "variable_declarator" {
			nameNode := parent.ChildByFieldName("name")
			if nameNode != nil {
				return string(source[nameNode.StartByte():nameNode.EndByte()])
			}
		}
	}
	if node.Type() == "lexical_declaration" {
		for idx := 0; idx < int(node.ChildCount()); idx++ {
			child := node.Child(idx)
			if child != nil && child.Type() == "variable_declarator" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					return string(source[nameNode.StartByte():nameNode.EndByte()])
				}
			}
		}
	}
	return ""
}
