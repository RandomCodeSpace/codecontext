// Package c implements a tree-sitter-based parser for C source files.
package c

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	clang "github.com/smacker/go-tree-sitter/c"
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
	Type      string // "function" | "struct" | "enum" | "union" | "type"
	Kind      string // "function" | "declaration" | "struct" | "enum" | "union" | "typedef"
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string
}

// Dependency represents an #include directive.
type Dependency struct {
	Path       string
	Type       string // "include"
	LineNumber int
	IsLocal    bool
}

// CParser is the entry point for C source files.
type CParser struct{}

// Parse parses a C source file using tree-sitter.
func (p *CParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	parser := sitter.NewParser()
	parser.SetLanguage(clang.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	WalkC(root, src, "", result)
	return result, nil
}

// WalkC recursively walks the tree-sitter AST extracting entities and dependencies.
// Exported so the C++ parser can reuse it for shared node types.
func WalkC(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "preproc_include":
			ExtractInclude(child, src, result)
		case "function_definition":
			ExtractFunction(child, src, parent, result)
		case "struct_specifier":
			ExtractStruct(child, src, parent, result)
		case "enum_specifier":
			ExtractEnum(child, src, parent, result)
		case "union_specifier":
			extractUnion(child, src, parent, result)
		case "type_definition":
			ExtractTypedef(child, src, result)
		case "declaration":
			ExtractDeclaration(child, src, parent, result)
		}
	}
}

// ExtractInclude handles #include directives.
func ExtractInclude(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "string_literal" || child.Type() == "system_lib_string" {
			raw := child.Content(src)
			isLocal := strings.Contains(raw, "\"")
			path := strings.Trim(raw, "\"<>")
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "include",
				LineNumber: lineNum,
				IsLocal:    isLocal,
			})
			return
		}
	}
}

// ExtractFunction extracts a function definition.
func ExtractFunction(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	name := ExtractDeclaratorName(node.ChildByFieldName("declarator"), src)
	if name == "" {
		return
	}
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entityType := "function"
	if parent != "" {
		entityType = "method"
	}

	sig := BuildSignature(node, src, name)
	docs := ExtractCComment(node, src)

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

// ExtractStruct extracts a struct definition and walks its body.
func ExtractStruct(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1
	docs := ExtractCComment(node, src)

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "struct",
		Kind:      "struct",
		StartLine: startLine,
		EndLine:   endLine,
		Docs:      docs,
		Parent:    parent,
	})

	body := node.ChildByFieldName("body")
	if body != nil {
		WalkC(body, src, name, result)
	}
}

// ExtractEnum extracts an enum definition.
func ExtractEnum(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1
	docs := ExtractCComment(node, src)

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

func extractUnion(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "union",
		Kind:      "union",
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parent,
	})
}

// ExtractTypedef extracts a typedef.
func ExtractTypedef(node *sitter.Node, src []byte, result *ParseResult) {
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

// ExtractDeclaration checks for function prototypes inside declarations.
func ExtractDeclaration(node *sitter.Node, src []byte, parent string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "function_declarator" {
			name := ExtractDeclaratorName(child, src)
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

// ExtractDeclaratorName walks through nested declarators to find the identifier name.
func ExtractDeclaratorName(node *sitter.Node, src []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "identifier", "field_identifier":
		return node.Content(src)
	case "function_declarator", "pointer_declarator":
		return ExtractDeclaratorName(node.ChildByFieldName("declarator"), src)
	case "parenthesized_declarator":
		if node.NamedChildCount() > 0 {
			return ExtractDeclaratorName(node.NamedChild(0), src)
		}
	case "qualified_identifier":
		// C++ qualified names like ClassName::methodName — return last part
		if node.NamedChildCount() > 0 {
			last := node.NamedChild(int(node.NamedChildCount()) - 1)
			return last.Content(src)
		}
	}
	return ""
}

// BuildSignature extracts the function signature (name + params).
func BuildSignature(node *sitter.Node, src []byte, name string) string {
	decl := node.ChildByFieldName("declarator")
	if decl == nil {
		return name
	}
	var funcDecl *sitter.Node
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "function_declarator" {
			funcDecl = n
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(decl)
	if funcDecl != nil {
		params := funcDecl.ChildByFieldName("parameters")
		if params != nil {
			return name + params.Content(src)
		}
	}
	return name
}

// ExtractCComment looks for a comment node immediately before the given node.
func ExtractCComment(node *sitter.Node, src []byte) string {
	prev := node.PrevSibling()
	if prev == nil {
		return ""
	}
	if prev.Type() != "comment" {
		return ""
	}
	text := prev.Content(src)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	return strings.TrimSpace(text)
}
