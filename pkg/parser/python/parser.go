// Package python implements a tree-sitter-based parser for Python source files.
package python

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

// Entity represents a named code element (function, method, class).
type Entity struct {
	Name      string
	Type      string // "function" | "method" | "class"
	Kind      string // "function" | "async_function" | "method" | "async_method" | "class"
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string // name of enclosing class, empty for top-level
}

// Dependency represents an import statement.
type Dependency struct {
	Path       string
	Type       string // "import" | "from"
	LineNumber int
	IsLocal    bool
}

// PythonParser is the entry point for Python source files.
type PythonParser struct{}

var pythonLang = grammars.PythonLanguage()

// Parse parses a Python source file using tree-sitter.
func (p *PythonParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	entry := grammars.DetectLanguage("x.py")
	parser := sitter.NewParser(pythonLang)

	var tree *sitter.Tree
	var err error
	if entry != nil && entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, pythonLang))
	} else {
		tree, err = parser.Parse(src)
	}
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	walkPython(root, src, "", result)
	return result, nil
}

// walkPython recursively walks the tree-sitter AST extracting entities and dependencies.
func walkPython(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type(pythonLang) {
		case "import_statement":
			extractPyImport(child, src, result)
		case "import_from_statement":
			extractPyFromImport(child, src, result)
		case "class_definition":
			extractPyClass(child, src, parentClass, result)
		case "function_definition":
			extractPyFunc(child, src, parentClass, false, result)
		case "decorated_definition":
			// Decorated definitions wrap the actual class/function.
			extractDecorated(child, src, parentClass, result)
		}
	}
}

func extractDecorated(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type(pythonLang) {
		case "class_definition":
			extractPyClass(child, src, parentClass, result)
		case "function_definition":
			extractPyFunc(child, src, parentClass, false, result)
		}
	}
}

// extractPyImport handles: import os, import sys as system
func extractPyImport(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	// Children are dotted_name or aliased_import nodes.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		var path string
		switch child.Type(pythonLang) {
		case "dotted_name":
			path = child.Text(src)
		case "aliased_import":
			// The first named child is the dotted_name.
			if child.NamedChildCount() > 0 {
				path = child.NamedChild(0).Text(src)
			}
		}
		if path != "" {
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "import",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
		}
	}
}

// extractPyFromImport handles: from typing import List, Optional
func extractPyFromImport(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	// Find the module_name child (the source module).
	var path string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type(pythonLang) == "dotted_name" || child.Type(pythonLang) == "relative_import" {
			path = child.Text(src)
			break
		}
	}
	if path != "" {
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path:       path,
			Type:       "from",
			LineNumber: lineNum,
			IsLocal:    strings.HasPrefix(path, "."),
		})
	}
}

// extractPyClass extracts a class and its methods.
func extractPyClass(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", pythonLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	docs := extractPyDocstring(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "class",
		Kind:      "class",
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parentClass,
	})

	// Walk class body for methods and nested classes.
	body := node.ChildByFieldName("body", pythonLang)
	if body != nil {
		walkPython(body, src, name, result)
	}
}

// extractPyFunc extracts a function or method.
func extractPyFunc(node *sitter.Node, src []byte, parentClass string, _ bool, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", pythonLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	// Determine if async by checking if parent is async or node text starts with "async"
	isAsync := false
	// Check if the parent node wraps this in an async context.
	// In tree-sitter-python, "async def" creates a function_definition node
	// but the first line text starts with "async".
	firstLine := node.Text(src)
	if idx := strings.IndexByte(firstLine, '\n'); idx > 0 {
		firstLine = firstLine[:idx]
	}
	if strings.HasPrefix(strings.TrimSpace(firstLine), "async ") {
		isAsync = true
	}

	entityType := "function"
	kind := "function"
	if parentClass != "" {
		entityType = "method"
		kind = "method"
	}
	if isAsync {
		kind = "async_" + kind
	}

	// Build signature from parameters.
	sig := name
	params := node.ChildByFieldName("parameters", pythonLang)
	if params != nil {
		sig = name + params.Text(src)
	}

	docs := extractPyDocstring(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      kind,
		Signature: sig,
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parentClass,
	})
}

// extractPyDocstring extracts the docstring from a class or function definition.
func extractPyDocstring(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body", pythonLang)
	if body == nil || body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first.Type(pythonLang) != "expression_statement" || first.NamedChildCount() == 0 {
		return ""
	}
	expr := first.NamedChild(0)
	if expr.Type(pythonLang) != "string" {
		return ""
	}
	raw := expr.Text(src)
	// Strip triple quotes.
	for _, delim := range []string{`"""`, `'''`} {
		if strings.HasPrefix(raw, delim) && strings.HasSuffix(raw, delim) {
			return strings.TrimSpace(raw[3 : len(raw)-3])
		}
	}
	// Strip single quotes.
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') {
		return strings.TrimSpace(raw[1 : len(raw)-1])
	}
	return ""
}
