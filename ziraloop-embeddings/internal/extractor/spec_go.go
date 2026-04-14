package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"
	golang "github.com/smacker/go-tree-sitter/golang"
)

func init() {
	Register(LanguageSpec{
		Language:   "go",
		TSLanguage: golang.GetLanguage(),
		Extensions: []string{".go"},
		FunctionTypes: toSet([]string{
			"function_declaration",
			"method_declaration",
		}),
		ClassTypes: toSet([]string{
			"type_declaration",
		}),
		NameField: "name",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			if node.Type() == "type_declaration" {
				// Only extract struct and interface types, skip simple aliases
				for idx := 0; idx < int(node.ChildCount()); idx++ {
					child := node.Child(idx)
					if child == nil || child.Type() != "type_spec" {
						continue
					}
					typeNode := child.ChildByFieldName("type")
					if typeNode == nil {
						return "" // skip — can't determine type kind
					}
					kind := typeNode.Type()
					if kind != "struct_type" && kind != "interface_type" {
						return "" // skip simple aliases, func types, etc.
					}
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						return string(source[nameNode.StartByte():nameNode.EndByte()])
					}
				}
			}
			return ""
		},
	})
}
