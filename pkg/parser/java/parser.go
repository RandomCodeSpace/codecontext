// Package java implements a tree-sitter-based parser for Java source files.
package java

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
)

// ParseResult is the output of Parse.
type ParseResult struct {
	FilePath     string
	Entities     []*Entity
	Dependencies []*Dependency
}

// Entity represents a named Java code element.
type Entity struct {
	Name      string
	Type      string // "class" | "interface" | "enum" | "annotation" | "method"
	Kind      string
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string // enclosing type name, or ""
}

// Dependency represents an import declaration.
type Dependency struct {
	Path       string
	Type       string // "import"
	LineNumber int
	IsLocal    bool
}

// JavaParser is the entry point.
type JavaParser struct{}

// Parse parses a Java source file using tree-sitter.
func (p *JavaParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	walkJava(root, src, "", result)
	return result, nil
}

// walkJava recursively walks the tree-sitter AST extracting entities and dependencies.
func walkJava(node *sitter.Node, src []byte, parentType string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		processJavaNode(child, src, parentType, result)
	}
}

func processJavaNode(node *sitter.Node, src []byte, parentType string, result *ParseResult) {
	switch node.Type() {
	case "import_declaration":
		extractJavaImport(node, src, result)
	case "class_declaration":
		extractJavaType(node, src, parentType, "class", result)
	case "interface_declaration":
		extractJavaType(node, src, parentType, "interface", result)
	case "enum_declaration":
		extractJavaType(node, src, parentType, "enum", result)
	case "annotation_type_declaration":
		extractJavaType(node, src, parentType, "annotation", result)
	case "method_declaration", "constructor_declaration":
		extractJavaMethod(node, src, parentType, result)
	}
}

// extractJavaImport handles: import java.util.List;
func extractJavaImport(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	// The import path is in a scoped_identifier or identifier child.
	text := node.Content(src)
	text = strings.TrimPrefix(text, "import ")
	text = strings.TrimPrefix(text, "static ")
	text = strings.TrimSuffix(strings.TrimSpace(text), ";")
	if text != "" {
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path:       text,
			Type:       "import",
			LineNumber: lineNum,
		})
	}
}

// extractJavaType extracts class, interface, enum, or annotation type declarations.
func extractJavaType(node *sitter.Node, src []byte, parentType, kind string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      kind,
		Kind:      kind,
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parentType,
	})

	// Walk body for methods and nested types.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkJavaBody(body, src, name, result)
	}
}

// walkJavaBody walks the body of a type declaration.
func walkJavaBody(body *sitter.Node, src []byte, parentType string, result *ParseResult) {
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		processJavaNode(child, src, parentType, result)
	}
}

// extractJavaMethod extracts a method or constructor declaration.
func extractJavaMethod(node *sitter.Node, src []byte, parentType string, result *ParseResult) {
	if parentType == "" {
		return
	}

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	// Build signature: return_type name(params)
	sig := name
	retType := node.ChildByFieldName("type")
	params := node.ChildByFieldName("parameters")
	if retType != nil {
		sig = retType.Content(src) + " " + name
	}
	if params != nil {
		sig += params.Content(src)
	} else {
		sig += "()"
	}

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "method",
		Kind:      "method",
		Signature: sig,
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parentType,
	})
}
