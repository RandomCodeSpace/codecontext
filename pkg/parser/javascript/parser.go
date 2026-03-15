// Package javascript implements a tree-sitter-based parser for JavaScript and
// TypeScript source files.
package javascript

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

// Entity represents a named code element.
type Entity struct {
	Name      string
	Type      string // "function" | "method" | "class"
	Kind      string // "function" | "async_function" | "arrow_function" | "async_arrow_function" | "method" | "async_method" | "class"
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string // enclosing class name, or ""
}

// Dependency represents an import or require statement.
type Dependency struct {
	Path       string
	Type       string // "import" | "require"
	LineNumber int
	IsLocal    bool
}

// JSParser is the entry point.
type JSParser struct{}

var javascriptLang = grammars.JavascriptLanguage()

// Parse parses a JavaScript or TypeScript source file using tree-sitter.
func (p *JSParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	src := []byte(content)
	entry := grammars.DetectLanguage("x.js")
	parser := sitter.NewParser(javascriptLang)

	var tree *sitter.Tree
	var err error
	if entry != nil && entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, javascriptLang))
	} else {
		tree, err = parser.Parse(src)
	}
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	walkJS(root, src, "", result)
	return result, nil
}

// walkJS recursively walks the tree-sitter AST.
func walkJS(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		processJSNode(child, src, parentClass, result)
	}
}

func processJSNode(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	switch node.Type(javascriptLang) {
	case "import_statement":
		extractJSImport(node, src, result)
	case "class_declaration":
		extractJSClass(node, src, parentClass, result)
	case "function_declaration":
		extractJSFuncDecl(node, src, parentClass, result)
	case "export_statement":
		extractJSExport(node, src, parentClass, result)
	case "lexical_declaration":
		// const/let arrow functions or requires
		extractJSLexicalDecl(node, src, parentClass, result)
	case "variable_declaration":
		// var arrow functions or requires
		extractJSVarDecl(node, src, parentClass, result)
	case "expression_statement":
		// require() calls
		extractJSRequireExpr(node, src, result)
	}
}

// extractJSImport handles: import X from 'path' and import 'path'
func extractJSImport(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	source := node.ChildByFieldName("source", javascriptLang)
	if source == nil {
		return
	}
	path := unquote(source.Text(src))
	if path != "" {
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path:       path,
			Type:       "import",
			LineNumber: lineNum,
			IsLocal:    strings.HasPrefix(path, "."),
		})
	}
}

// extractJSClass extracts a class and its methods.
func extractJSClass(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", javascriptLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "class",
		Kind:      "class",
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parentClass,
	})

	// Walk class body for methods.
	body := node.ChildByFieldName("body", javascriptLang)
	if body != nil {
		extractClassMethods(body, src, name, result)
	}
}

// extractClassMethods extracts methods from a class body.
func extractClassMethods(body *sitter.Node, src []byte, className string, result *ParseResult) {
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type(javascriptLang) {
		case "method_definition":
			extractJSMethod(child, src, className, result)
		}
	}
}

// extractJSMethod extracts a method from a class body.
func extractJSMethod(node *sitter.Node, src []byte, className string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", javascriptLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	// Check for async by inspecting the node text.
	isAsync := false
	nodeText := node.Text(src)
	if idx := strings.IndexByte(nodeText, '\n'); idx > 0 {
		nodeText = nodeText[:idx]
	}
	trimmed := strings.TrimSpace(nodeText)
	if strings.HasPrefix(trimmed, "async ") {
		isAsync = true
	}

	kind := "method"
	if isAsync {
		kind = "async_method"
	}

	// Build signature.
	sig := name
	params := node.ChildByFieldName("parameters", javascriptLang)
	if params != nil {
		sig = name + params.Text(src)
	}

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      "method",
		Kind:      kind,
		Signature: sig,
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    className,
	})
}

// extractJSFuncDecl handles: [async] function name(params)
func extractJSFuncDecl(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", javascriptLang)
	if nameNode == nil {
		return
	}
	name := nameNode.Text(src)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	// Check async.
	isAsync := false
	nodeText := node.Text(src)
	if idx := strings.IndexByte(nodeText, '\n'); idx > 0 {
		nodeText = nodeText[:idx]
	}
	if strings.Contains(strings.TrimSpace(nodeText), "async function") {
		isAsync = true
	}

	entityType := "function"
	kind := "function"
	if isAsync {
		kind = "async_function"
	}
	if parentClass != "" {
		entityType = "method"
		kind = "method"
		if isAsync {
			kind = "async_method"
		}
	}

	sig := name
	params := node.ChildByFieldName("parameters", javascriptLang)
	if params != nil {
		sig = name + params.Text(src)
	}

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      kind,
		Signature: sig,
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parentClass,
	})
}

// extractJSExport handles export statements by unwrapping them.
func extractJSExport(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	// Export wraps declarations. Find the inner declaration.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type(javascriptLang) {
		case "class_declaration":
			extractJSClass(child, src, parentClass, result)
		case "function_declaration":
			extractJSFuncDecl(child, src, parentClass, result)
		case "lexical_declaration":
			extractJSLexicalDecl(child, src, parentClass, result)
		}
	}
}

// extractJSLexicalDecl handles: const/let name = [async] (params) => ...
func extractJSLexicalDecl(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type(javascriptLang) != "variable_declarator" {
			continue
		}
		extractVarDeclarator(child, src, parentClass, result)
	}
}

// extractJSVarDecl handles: var name = [async] (params) => ...
func extractJSVarDecl(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type(javascriptLang) != "variable_declarator" {
			continue
		}
		extractVarDeclarator(child, src, parentClass, result)
	}
}

func extractVarDeclarator(node *sitter.Node, src []byte, parentClass string, result *ParseResult) {
	nameNode := node.ChildByFieldName("name", javascriptLang)
	valueNode := node.ChildByFieldName("value", javascriptLang)
	if nameNode == nil || valueNode == nil {
		return
	}

	name := nameNode.Text(src)

	// Check if value is an arrow function.
	val := valueNode
	isAsync := false
	if val.Type(javascriptLang) == "await_expression" || val.Type(javascriptLang) == "call_expression" {
		// Check for require() calls
		extractRequireFromValue(val, src, int(node.StartPoint().Row)+1, result)
		return
	}
	if val.Type(javascriptLang) == "arrow_function" {
		// Arrow function.
	} else {
		// Check for require() in the value.
		extractRequireFromValue(val, src, int(node.StartPoint().Row)+1, result)
		return
	}

	// Check for async in the node text.
	declText := node.Text(src)
	if idx := strings.IndexByte(declText, '\n'); idx > 0 {
		declText = declText[:idx]
	}
	if strings.Contains(declText, "async") {
		isAsync = true
	}

	startLine := int(node.Parent().StartPoint().Row) + 1
	endLine := int(node.Parent().EndPoint().Row) + 1

	entityType := "function"
	kind := "arrow_function"
	if isAsync {
		kind = "async_arrow_function"
	}
	if parentClass != "" {
		entityType = "method"
	}

	result.Entities = append(result.Entities, &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      kind,
		Signature: name,
		StartLine: startLine,
		EndLine:   endLine,
		Parent:    parentClass,
	})
}

// extractRequireFromValue looks for require('path') calls in a value node.
func extractRequireFromValue(node *sitter.Node, src []byte, lineNum int, result *ParseResult) {
	if node.Type(javascriptLang) == "call_expression" {
		fn := node.ChildByFieldName("function", javascriptLang)
		if fn != nil && fn.Text(src) == "require" {
			args := node.ChildByFieldName("arguments", javascriptLang)
			if args != nil && args.NamedChildCount() > 0 {
				arg := args.NamedChild(0)
				path := unquote(arg.Text(src))
				if path != "" {
					result.Dependencies = append(result.Dependencies, &Dependency{
						Path:       path,
						Type:       "require",
						LineNumber: lineNum,
						IsLocal:    strings.HasPrefix(path, "."),
					})
				}
			}
		}
	}
}

// extractJSRequireExpr handles: require('path') as a standalone expression.
func extractJSRequireExpr(node *sitter.Node, src []byte, result *ParseResult) {
	lineNum := int(node.StartPoint().Row) + 1
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		extractRequireFromValue(child, src, lineNum, result)
	}
}

// unquote removes surrounding quotes from a string literal.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return ""
	}
	if (s[0] == '\'' || s[0] == '"' || s[0] == '`') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return ""
}
