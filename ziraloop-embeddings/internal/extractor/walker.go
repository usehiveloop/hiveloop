package extractor

import (
	"context"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/ziraloop/ziraloop-embeddings/internal/model"
)

// ExtractSymbols parses source bytes and extracts symbols using the language spec.
func ExtractSymbols(filePath string, source []byte, spec *LanguageSpec, repoRoot string) []model.Symbol {
	parser := sitter.NewParser()
	parser.SetLanguage(spec.TSLanguage)

	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil
	}
	defer tree.Close()

	relPath, _ := filepath.Rel(repoRoot, filePath)
	if relPath == "" {
		relPath = filePath
	}

	var symbols []model.Symbol
	walkNode(tree.RootNode(), source, spec, relPath, spec.Language, &symbols)
	return symbols
}

func walkNode(node *sitter.Node, source []byte, spec *LanguageSpec, filePath, language string, symbols *[]model.Symbol) {
	nodeType := node.Type()

	if spec.FunctionTypes[nodeType] || spec.ClassTypes[nodeType] {
		name := extractName(node, source, spec)
		if name == "" || len(name) < 2 {
			goto children
		}

		body := source[node.StartByte():node.EndByte()]
		bodyStr := string(body)

		if len(bodyStr) < 10 {
			goto children
		}

		embedText := bodyStr
		if len(embedText) > 3000 {
			embedText = embedText[:3000]
		}

		*symbols = append(*symbols, model.Symbol{
			Name:      name,
			FilePath:  filePath,
			StartLine: int(node.StartPoint().Row) + 1,
			EndLine:   int(node.EndPoint().Row) + 1,
			NodeType:  nodeType,
			Language:  language,
			Body:      bodyStr,
			EmbedText: embedText,
		})

		// Don't recurse into function bodies — we only want top-level symbols
		// But DO recurse into class/type bodies to find methods
		if spec.ClassTypes[nodeType] {
			goto children
		}
		return
	}

children:
	for idx := 0; idx < int(node.ChildCount()); idx++ {
		child := node.Child(idx)
		if child != nil {
			walkNode(child, source, spec, filePath, language, symbols)
		}
	}
}

func extractName(node *sitter.Node, source []byte, spec *LanguageSpec) string {
	// Try the spec's custom extractor first
	if spec.NameExtractor != nil {
		if name := spec.NameExtractor(node, source); name != "" {
			return name
		}
	}

	// Try the standard name field
	nameNode := node.ChildByFieldName(spec.NameField)
	if nameNode != nil {
		return string(source[nameNode.StartByte():nameNode.EndByte()])
	}

	// Try common field names as fallback
	for _, field := range []string{"name", "declarator", "pattern"} {
		nameNode = node.ChildByFieldName(field)
		if nameNode != nil {
			return string(source[nameNode.StartByte():nameNode.EndByte()])
		}
	}

	return ""
}
