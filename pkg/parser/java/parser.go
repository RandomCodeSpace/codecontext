// Package java implements a tree-sitter-based parser for Java source files.
package java

import (
	"strings"

	sitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
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

var javaLang = grammars.JavaLanguage()

// Parse parses a Java source file using tree-sitter.
func (p *JavaParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	entry := grammars.DetectLanguage("x.java")
	parser := sitter.NewParser(javaLang)

	var tree *sitter.Tree
	var err error
	if entry != nil && entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, javaLang))
	} else {
		tree, err = parser.Parse(src)
	}
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
	switch node.Type(javaLang) {
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
	text := node.Text(src)
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
	nameNode := node.ChildByFieldName("name", javaLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
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
	body := node.ChildByFieldName("body", javaLang)
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

	nameNode := node.ChildByFieldName("name", javaLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	// Build signature: return_type name(params)
	sig := name
	retType := node.ChildByFieldName("type", javaLang)
	params := node.ChildByFieldName("parameters", javaLang)
	if retType != nil {
		sig = retType.Text(src) + " " + name
	}
	if params != nil {
		sig += params.Text(src)
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
