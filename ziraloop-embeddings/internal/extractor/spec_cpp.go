package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"
	cpp "github.com/smacker/go-tree-sitter/cpp"
	c "github.com/smacker/go-tree-sitter/c"
)

func init() {
	Register(LanguageSpec{
		Language:   "cpp",
		TSLanguage: cpp.GetLanguage(),
		Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hxx"},
		FunctionTypes: toSet([]string{
			"function_definition",
		}),
		ClassTypes: toSet([]string{
			"class_specifier",
			"struct_specifier",
			"enum_specifier",
			"namespace_definition",
		}),
		NameField: "declarator",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			return cppNameExtractor(node, source)
		},
	})

	Register(LanguageSpec{
		Language:   "c",
		TSLanguage: c.GetLanguage(),
		Extensions: []string{".c", ".h"},
		FunctionTypes: toSet([]string{
			"function_definition",
		}),
		ClassTypes: toSet([]string{
			"struct_specifier",
			"enum_specifier",
			"union_specifier",
		}),
		NameField: "declarator",
		NameExtractor: func(node *sitter.Node, source []byte) string {
			return cppNameExtractor(node, source)
		},
	})
}

// cppNameExtractor handles C/C++ declarator nesting.
func cppNameExtractor(node *sitter.Node, source []byte) string {
	// For function_definition, the name is inside a declarator chain
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil {
		return ""
	}
	return extractDeclaratorName(declarator, source, 0)
}

func extractDeclaratorName(node *sitter.Node, source []byte, depth int) string {
	if depth > 8 {
		return ""
	}

	switch node.Type() {
	case "identifier", "field_identifier", "destructor_name", "operator_name":
		return string(source[node.StartByte():node.EndByte()])
	case "qualified_identifier", "template_function":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(source[nameNode.StartByte():nameNode.EndByte()])
		}
	case "function_declarator", "pointer_declarator", "reference_declarator",
		"parenthesized_declarator", "array_declarator":
		inner := node.ChildByFieldName("declarator")
		if inner != nil {
			return extractDeclaratorName(inner, source, depth+1)
		}
	}

	// Try first child as fallback
	if node.ChildCount() > 0 {
		return extractDeclaratorName(node.Child(0), source, depth+1)
	}
	return ""
}
