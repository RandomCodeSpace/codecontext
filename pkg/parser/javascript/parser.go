// Package javascript implements an AST-quality parser for JavaScript and
// TypeScript source files.
//
// The parser works in two passes:
//  1. Tokenise: scan the raw source character-by-character, replacing the
//     content of string literals, template literals and comments with spaces
//     while preserving all newlines.  This guarantees that keywords appearing
//     inside strings or comments are never misidentified as declarations.
//  2. Analyse: process the tokenised lines to detect declarations (functions,
//     classes, arrow functions, methods) and import/require statements.
//     Brace-depth tracking lets us record accurate EndLine values and build
//     correct parent/child relationships for class methods.
package javascript

import (
	"strings"
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

// --------------------------------------------------------------------------
// Pass 1 – tokeniser
// --------------------------------------------------------------------------

// cleanSource returns a copy of src where the *content* of all string
// literals, template literals and comments is replaced with spaces.
// Newlines are always preserved so line numbers remain correct.
func cleanSource(src string) string {
	out := []byte(src)
	i := 0
	n := len(src)

	blank := func(start, end int) {
		for k := start; k < end && k < n; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}

	for i < n {
		// Single-line comment.
		if i+1 < n && src[i] == '/' && src[i+1] == '/' {
			j := i + 2
			for j < n && src[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
			continue
		}

		// Block comment.
		if i+1 < n && src[i] == '/' && src[i+1] == '*' {
			j := i + 2
			for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			blank(i, j+2)
			i = j + 2
			continue
		}

		// Template literal (backtick).  We handle nesting naively – nested
		// ${…} expressions are blanked together with the rest of the literal.
		if src[i] == '`' {
			j := i + 1
			for j < n {
				if src[j] == '`' {
					j++
					break
				}
				if src[j] == '\\' {
					j += 2
					continue
				}
				j++
			}
			blank(i, j)
			i = j
			continue
		}

		// Single- or double-quoted string.
		if src[i] == '\'' || src[i] == '"' {
			q := src[i]
			j := i + 1
			for j < n && src[j] != '\n' {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == q {
					j++
					break
				}
				j++
			}
			blank(i, j)
			i = j
			continue
		}

		i++
	}

	return string(out)
}

// --------------------------------------------------------------------------
// Pass 2 – structural analyser
// --------------------------------------------------------------------------

// braceFrame is pushed onto the scope stack when we open a brace.
type braceFrame struct {
	openLine  int
	entityIdx int  // index in result.Entities, or -1
	isClass   bool // true when this brace belongs to a class body
	className string
}

// JSParser is the entry point.
type JSParser struct{}

// Parse parses a JavaScript or TypeScript source file.
func (p *JSParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	clean := cleanSource(content)
	lines := strings.Split(clean, "\n")
	rawLines := strings.Split(content, "\n")

	// ---- brace-depth tracking ----
	var braceStack []braceFrame

	// currentClass returns the enclosing class name, or "".
	currentClass := func() string {
		for i := len(braceStack) - 1; i >= 0; i-- {
			if braceStack[i].isClass {
				return braceStack[i].className
			}
		}
		return ""
	}

	// insideClassBody is true when the immediately enclosing brace is a class.
	insideClassBody := func() bool {
		if len(braceStack) == 0 {
			return false
		}
		return braceStack[len(braceStack)-1].isClass
	}

	// We process the cleaned source line-by-line, but also track character-
	// level brace positions so we can push/pop the stack correctly.
	lineEntityIdx := -1  // entity opened on the current line
	lineIsClass := false // is the entity on the current line a class?
	linePendingClass := ""

	// pendingImport holds an import statement that may span multiple lines.
	// (We handle it simply as a single-line match for the common case.)

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)
		rawTrimmed := ""
		if lineIdx < len(rawLines) {
			rawTrimmed = strings.TrimSpace(rawLines[lineIdx])
		}

		lineEntityIdx = -1
		lineIsClass = false
		linePendingClass = ""

		// ---- imports ----
		// Use raw line for import/require so string content is not blanked.
		if imp, ok := extractImport(rawTrimmed, lineNum); ok {
			result.Dependencies = append(result.Dependencies, imp)
		}
		for _, req := range extractRequires(rawTrimmed, lineNum) {
			result.Dependencies = append(result.Dependencies, req)
		}

		// ---- class declaration ----
		if name, ok := matchClassDecl(trimmed); ok {
			parent := currentClass()
			entityIdx := len(result.Entities)
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      "class",
				Kind:      "class",
				StartLine: lineNum,
				EndLine:   lineNum,
				Parent:    parent,
			})
			lineEntityIdx = entityIdx
			lineIsClass = true
			linePendingClass = name
		}

		// ---- function declaration: function [async] name(params) ----
		if lineEntityIdx < 0 {
			if ent, ok := matchFuncDecl(trimmed, currentClass()); ok {
				ent.StartLine = lineNum
				ent.EndLine = lineNum
				entityIdx := len(result.Entities)
				result.Entities = append(result.Entities, ent)
				lineEntityIdx = entityIdx
			}
		}

		// ---- arrow function: const/let/var name = [async] ([params]) => ----
		if lineEntityIdx < 0 {
			if ent, ok := matchArrowFunc(trimmed, currentClass()); ok {
				ent.StartLine = lineNum
				ent.EndLine = lineNum
				entityIdx := len(result.Entities)
				result.Entities = append(result.Entities, ent)
				lineEntityIdx = entityIdx
			}
		}

		// ---- method inside class body ----
		if lineEntityIdx < 0 && insideClassBody() {
			if ent, ok := matchMethod(trimmed, rawTrimmed, currentClass()); ok {
				ent.StartLine = lineNum
				ent.EndLine = lineNum
				entityIdx := len(result.Entities)
				result.Entities = append(result.Entities, ent)
				lineEntityIdx = entityIdx
			}
		}

		// ---- brace tracking ----
		// Count braces on this line to maintain the scope stack.
		for _, ch := range line {
			if ch == '{' {
				frame := braceFrame{
					openLine:  lineNum,
					entityIdx: lineEntityIdx,
					isClass:   lineIsClass,
					className: linePendingClass,
				}
				braceStack = append(braceStack, frame)
				// After the first { on this line, subsequent { belong to no entity.
				lineEntityIdx = -1
				lineIsClass = false
				linePendingClass = ""
			} else if ch == '}' {
				if len(braceStack) > 0 {
					frame := braceStack[len(braceStack)-1]
					braceStack = braceStack[:len(braceStack)-1]
					if frame.entityIdx >= 0 && frame.entityIdx < len(result.Entities) {
						result.Entities[frame.entityIdx].EndLine = lineNum
					}
				}
			}
		}
	}

	// Close any still-open entities at EOF.
	lastLine := len(lines)
	for _, frame := range braceStack {
		if frame.entityIdx >= 0 && frame.entityIdx < len(result.Entities) {
			result.Entities[frame.entityIdx].EndLine = lastLine
		}
	}

	return result, nil
}

// --------------------------------------------------------------------------
// Declaration matchers
// --------------------------------------------------------------------------

// matchClassDecl matches: [export] [default] class Name
func matchClassDecl(line string) (name string, ok bool) {
	s := skipExportDefault(line)
	if !strings.HasPrefix(s, "class ") {
		return "", false
	}
	rest := strings.TrimSpace(s[6:])
	name = firstIdent(rest)
	if name == "" {
		return "", false
	}
	return name, true
}

// matchFuncDecl matches top-level function declarations:
//
//	[export] [default] [async] function name(params) {
func matchFuncDecl(line, parentClass string) (*Entity, bool) {
	s := skipExportDefault(line)
	isAsync := false
	if strings.HasPrefix(s, "async ") {
		s = strings.TrimSpace(s[6:])
		isAsync = true
	}
	if !strings.HasPrefix(s, "function") {
		return nil, false
	}
	rest := strings.TrimSpace(s[8:])
	// Named function: next non-space chars are the name.
	// Anonymous: rest starts with '(' – skip.
	name := firstIdent(rest)
	if name == "" {
		return nil, false
	}
	sig := buildSig(name, rest)
	kind := "function"
	entityType := "function"
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
	return &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      kind,
		Signature: sig,
		Parent:    parentClass,
	}, true
}

// matchArrowFunc matches:
//
//	[export] const/let/var name = [async] ([params]) =>
func matchArrowFunc(line, parentClass string) (*Entity, bool) {
	s := skipExportDefault(line)

	// Must start with const/let/var.
	var rest string
	for _, kw := range []string{"const ", "let ", "var "} {
		if strings.HasPrefix(s, kw) {
			rest = strings.TrimSpace(s[len(kw):])
			break
		}
	}
	if rest == "" {
		return nil, false
	}

	// name = ...
	eqIdx := strings.IndexByte(rest, '=')
	if eqIdx < 0 {
		return nil, false
	}
	name := strings.TrimSpace(rest[:eqIdx])
	if name == "" || strings.ContainsAny(name, "({[") {
		return nil, false
	}

	after := strings.TrimSpace(rest[eqIdx+1:])
	isAsync := false
	if strings.HasPrefix(after, "async ") {
		isAsync = true
		after = strings.TrimSpace(after[6:])
	}

	// Must contain => somewhere.
	if !strings.Contains(after, "=>") {
		return nil, false
	}

	kind := "arrow_function"
	entityType := "function"
	if isAsync {
		kind = "async_arrow_function"
	}
	if parentClass != "" {
		entityType = "method"
	}
	return &Entity{
		Name:      name,
		Type:      entityType,
		Kind:      kind,
		Signature: name,
		Parent:    parentClass,
	}, true
}

// matchMethod matches method declarations inside a class body.
// It accepts both the cleaned and raw trimmed versions (the raw version is
// used only for the signature string).
//
// Patterns handled:
//
//	[static] [async] [get|set] name(params) {
//	[static] [async] * name(params) {   (generators)
func matchMethod(line, rawLine, className string) (*Entity, bool) {
	if className == "" {
		return nil, false
	}
	s := line

	// Skip static/async/get/set/generator prefixes.
	isAsync := false
	for {
		changed := false
		for _, kw := range []string{"static ", "async ", "get ", "set ", "* "} {
			if strings.HasPrefix(s, kw) {
				if kw == "async " {
					isAsync = true
				}
				s = strings.TrimSpace(s[len(kw):])
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	name := firstIdent(s)
	if name == "" {
		return nil, false
	}
	// Skip JS keywords / built-in names that look like method names.
	switch name {
	case "if", "for", "while", "switch", "return", "new",
		"throw", "try", "catch", "finally", "else":
		return nil, false
	}

	// After name must come '('.
	after := strings.TrimSpace(s[len(name):])
	if !strings.HasPrefix(after, "(") {
		return nil, false
	}

	sig := buildSig(name, s)
	kind := "method"
	if isAsync {
		kind = "async_method"
	}
	return &Entity{
		Name:      name,
		Type:      "method",
		Kind:      kind,
		Signature: sig,
		Parent:    className,
	}, true
}

// --------------------------------------------------------------------------
// Import helpers
// --------------------------------------------------------------------------

// extractImport handles:  import ... from 'path'  and  import 'path'
func extractImport(line string, lineNum int) (*Dependency, bool) {
	s := skipExportDefault(line)
	if !strings.HasPrefix(s, "import ") {
		return nil, false
	}
	// Find from 'path' or "path"
	if idx := strings.LastIndex(s, " from "); idx >= 0 {
		pathPart := strings.TrimSpace(s[idx+6:])
		path := unquote(pathPart)
		if path == "" {
			return nil, false
		}
		return &Dependency{
			Path:       path,
			Type:       "import",
			LineNumber: lineNum,
			IsLocal:    strings.HasPrefix(path, "."),
		}, true
	}
	// bare import 'path'
	rest := strings.TrimSpace(s[7:])
	path := unquote(rest)
	if path != "" {
		return &Dependency{
			Path:       path,
			Type:       "import",
			LineNumber: lineNum,
			IsLocal:    strings.HasPrefix(path, "."),
		}, true
	}
	return nil, false
}

// extractRequires finds all require('path') calls on a line.
func extractRequires(line string, lineNum int) []*Dependency {
	var deps []*Dependency
	s := line
	for {
		idx := strings.Index(s, "require(")
		if idx < 0 {
			break
		}
		rest := s[idx+8:]
		path := unquote(strings.TrimSpace(rest))
		if path != "" {
			deps = append(deps, &Dependency{
				Path:       path,
				Type:       "require",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
		}
		s = rest
	}
	return deps
}

// --------------------------------------------------------------------------
// Small utilities
// --------------------------------------------------------------------------

// skipExportDefault strips leading "export ", "export default " tokens.
func skipExportDefault(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "export ") {
		s = strings.TrimSpace(s[7:])
	}
	if strings.HasPrefix(s, "default ") {
		s = strings.TrimSpace(s[8:])
	}
	return s
}

// firstIdent returns the leading identifier from s (letters, digits, _$).
func firstIdent(s string) string {
	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '$' {
			end++
		} else {
			break
		}
	}
	return s[:end]
}

// buildSig builds "name(params)" from a fragment starting with name.
func buildSig(name, fragment string) string {
	after := fragment[len(name):]
	pOpen := strings.IndexByte(after, '(')
	if pOpen < 0 {
		return name + "()"
	}
	depth := 0
	for i := pOpen; i < len(after); i++ {
		switch after[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return name + after[pOpen:i+1]
			}
		}
	}
	return name + after[pOpen:]
}

// unquote removes surrounding single or double quotes from a string literal.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return ""
	}
	if (s[0] == '\'' && s[len(s)-1] == '\'') ||
		(s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	// Might end with '; or ',  — strip trailing punctuation.
	for _, q := range []byte{'\'', '"', ';', ','} {
		if s[0] == '\'' || s[0] == '"' {
			end := strings.IndexByte(s[1:], q)
			if end >= 0 {
				return s[1 : end+1]
			}
		}
	}
	return ""
}
