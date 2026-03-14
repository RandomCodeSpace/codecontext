// Package cpp implements a tree-sitter-based parser for C++ source files.
// It reuses helper functions from the C parser for shared constructs.
package cpp

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	cpplang "github.com/smacker/go-tree-sitter/cpp"

	cparser "github.com/RandomCodeSpace/codecontext/pkg/parser/c"
)

// ParseResult is the output of Parse.
type ParseResult struct {
	FilePath     string
	Entities     []*Entity
	Dependencies []*Dependency
}

// Entity represents a named code element.
type Entity struct {
	Name      string
	Type      string // "function" | "method" | "class" | "struct" | "enum" | "namespace" | "type"
	Kind      string
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string
}

// Dependency represents an #include or using directive.
type Dependency struct {
	Path       string
	Type       string // "include" | "using"
	LineNumber int
	IsLocal    bool
}

// CppParser is the entry point for C++ source files.
type CppParser struct{}

// Parse parses a C++ source file using tree-sitter.
func (p *CppParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	parser := sitter.NewParser()
	parser.SetLanguage(cpplang.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	walkCpp(root, src, "", result)
	return result, nil
}

func walkCpp(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "preproc_include":
			extractInclude(child, src, result)
		case "function_definition":
			extractFunction(child, src, parent, result)
		case "struct_specifier":
			extractStructOrClass(child, src, parent, "struct", result)
		case "class_specifier":
			extractStructOrClass(child, src, parent, "class", result)
		case "enum_specifier":
			extractEnum(child, src, parent, result)
		case "namespace_definition":
			extractNamespace(child, src, result)
		case "template_declaration":
			// Templates wrap a class/function — walk into them
			walkCpp(child, src, parent, result)
		case "type_definition":
			extractTypedef(child, src, result)
		case "using_declaration":
			extractUsing(child, src, result)
		case "declaration":
			extractDeclaration(child, src, parent, result)
		}
	}
}

// ── Helpers that delegate to the C parser's exported functions ──

func extractInclude(node *sitter.Node, src []byte, result *ParseResult) {
	cResult := &cparser.ParseResult{}
	cparser.ExtractInclude(node, src, cResult)
	for _, d := range cResult.Dependencies {
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		})
	}
}

func extractFunction(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	name := cparser.ExtractDeclaratorName(node.ChildByFieldName("declarator"), src)
	if name == "" {
		return
	}
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entityType := "function"
	if parent != "" {
		entityType = "method"
	}

	sig := cparser.BuildSignature(node, src, name)
	docs := cparser.ExtractCComment(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      entityType,
		Signature: sig,
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parent,
	})
}

func extractStructOrClass(node *sitter.Node, src []byte, parent string, kind string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1
	docs := cparser.ExtractCComment(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      kind,
		Kind:      kind,
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parent,
	})

	// Walk body for methods, nested classes, etc.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkCpp(body, src, name, result)
	}
}

func extractEnum(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1
	docs := cparser.ExtractCComment(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "enum",
		Kind:      "enum",
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parent,
	})
}

func extractNamespace(node *sitter.Node, src []byte, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return // anonymous namespace
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "namespace",
		Kind:      "namespace",
		StartLine: startLine,
		EndLine:   endLine,
	})

	// Walk namespace body
	body := node.ChildByFieldName("body")
	if body != nil {
		walkCpp(body, src, name, result)
	}
}

func extractTypedef(node *sitter.Node, src []byte, result *ParseResult) {
	var name string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			name = child.Content(src)
		}
	}
	if name == "" {
		return
	}
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "type",
		Kind:      "typedef",
		StartLine: startLine,
		EndLine:   endLine,
	})
}

func extractUsing(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	text := node.Content(src)
	// Extract the path from "using namespace std;" or "using std::vector;"
	text = strings.TrimPrefix(text, "using")
	text = strings.TrimPrefix(text, " namespace")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)
	if text != "" {
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path:       text,
			Type:       "using",
			LineNumber: lineNum,
			IsLocal:    false,
		})
	}
}

func extractDeclaration(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "function_declarator" {
			name := cparser.ExtractDeclaratorName(child, src)
			if name != "" {
				startLine := int(node.StartPoint().Row) + 1
				result.Entities = append(result.Entities, &Entity{
					Name:      name,
					Type:      "function",
					Kind:      "declaration",
					StartLine: startLine,
					EndLine:   startLine,
					Parent:    parent,
				})
			}
		}
	}
}
