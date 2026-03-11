// Package java implements an AST-quality parser for Java source files.
//
// The parser works in two passes identical in principle to the JavaScript parser:
//  1. Tokenise: replace the content of string/char literals and comments with
//     spaces, preserving all newlines so line numbers remain correct.
//  2. Analyse: scan the tokenised lines to detect class, interface, enum,
//     method, and import declarations.  Brace-depth tracking is used to
//     compute accurate EndLine values and parent/child relationships.
package java

import (
	"strings"
	"unicode"
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

// --------------------------------------------------------------------------
// Pass 1 – tokeniser
// --------------------------------------------------------------------------

// cleanSource replaces the content of Java string literals, char literals,
// and comments (// and /* */) with spaces.  Newlines are preserved.
func cleanSource(src string) string {
	out := []byte(src)
	n := len(src)
	i := 0

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
		// Block comment (including Javadoc /** ... */).
		if i+1 < n && src[i] == '/' && src[i+1] == '*' {
			j := i + 2
			for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			blank(i, j+2)
			i = j + 2
			continue
		}
		// String literal.
		if src[i] == '"' {
			j := i + 1
			for j < n && src[j] != '\n' {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '"' {
					j++
					break
				}
				j++
			}
			blank(i, j)
			i = j
			continue
		}
		// Char literal.
		if src[i] == '\'' {
			j := i + 1
			for j < n && src[j] != '\n' {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '\'' {
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

// braceFrame records what opened a '{'.
type braceFrame struct {
	openLine  int
	entityIdx int  // index in result.Entities, or -1
	isType    bool // true when brace belongs to a class/interface/enum body
	typeName  string
}

// JavaParser is the entry point.
type JavaParser struct{}

// Parse parses a Java source file.
func (p *JavaParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	clean := cleanSource(content)
	lines := strings.Split(clean, "\n")

	var braceStack []braceFrame

	// currentType returns the name of the innermost enclosing type, or "".
	currentType := func() string {
		for i := len(braceStack) - 1; i >= 0; i-- {
			if braceStack[i].isType {
				return braceStack[i].typeName
			}
		}
		return ""
	}

	// insideTypeBody is true when the immediately enclosing '{' is a type body.
	insideTypeBody := func() bool {
		if len(braceStack) == 0 {
			return false
		}
		return braceStack[len(braceStack)-1].isType
	}

	// pendingEntityIdx/pendingIsType/pendingTypeName carry a declaration forward
	// across lines until its opening '{' is found.  This handles multi-line
	// declarations such as:
	//
	//   public class Foo
	//       extends Bar
	//       implements Baz {   ← '{' is here, not on the "class" line
	//
	// Without carry-forward the brace frame would be pushed with isType=false,
	// making insideTypeBody() return false for the entire class body and causing
	// zero methods (and therefore zero relations) to be detected.
	pendingEntityIdx := -1
	pendingIsType := false
	pendingTypeName := ""

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		trimmed := strings.TrimSpace(line)

		// ---- import declaration ----
		if strings.HasPrefix(trimmed, "import ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "import "))
			rest = strings.TrimPrefix(rest, "static ")
			path := strings.TrimSuffix(strings.TrimSpace(rest), ";")
			if path != "" {
				result.Dependencies = append(result.Dependencies, &Dependency{
					Path:       path,
					Type:       "import",
					LineNumber: lineNum,
				})
			}
			continue
		}

		// ---- package declaration (skip) ----
		if strings.HasPrefix(trimmed, "package ") {
			continue
		}

		// ---- type declarations: class / interface / enum / @interface ----
		if name, kind, ok := matchTypeDecl(trimmed); ok {
			parent := currentType()
			entityIdx := len(result.Entities)
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      kind,
				Kind:      kind,
				StartLine: lineNum,
				EndLine:   lineNum,
				Parent:    parent,
			})
			pendingEntityIdx = entityIdx
			pendingIsType = true
			pendingTypeName = name
		} else if insideTypeBody() {
			// ---- method declarations (only inside a type body) ----
			if ent, ok := matchMethodDecl(trimmed, currentType()); ok {
				ent.StartLine = lineNum
				ent.EndLine = lineNum
				entityIdx := len(result.Entities)
				result.Entities = append(result.Entities, ent)
				pendingEntityIdx = entityIdx
				pendingIsType = false
				pendingTypeName = ""
			}
		}

		// ---- brace / semicolon tracking ----
		for _, ch := range line {
			if ch == '{' {
				frame := braceFrame{
					openLine:  lineNum,
					entityIdx: pendingEntityIdx,
					isType:    pendingIsType,
					typeName:  pendingTypeName,
				}
				braceStack = append(braceStack, frame)
				// Consume the pending declaration; subsequent '{' on the same
				// line (e.g. anonymous blocks) are unrelated.
				pendingEntityIdx = -1
				pendingIsType = false
				pendingTypeName = ""
			} else if ch == '}' {
				if len(braceStack) > 0 {
					frame := braceStack[len(braceStack)-1]
					braceStack = braceStack[:len(braceStack)-1]
					if frame.entityIdx >= 0 && frame.entityIdx < len(result.Entities) {
						result.Entities[frame.entityIdx].EndLine = lineNum
					}
				}
			} else if ch == ';' {
				// A ';' before any '{' on this line means the pending
				// declaration has no body (abstract method, interface method
				// signature, field).  Clear the pending state so we do not
				// accidentally attach a later '{' to it.
				pendingEntityIdx = -1
				pendingIsType = false
				pendingTypeName = ""
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

// matchTypeDecl recognises class / interface / enum / @interface declarations.
func matchTypeDecl(line string) (name, kind string, ok bool) {
	tokens := tokeniseJava(line)
	for i, tok := range tokens {
		switch tok {
		case "class":
			if i+1 < len(tokens) && tokens[i+1] != "(" {
				return tokens[i+1], "class", true
			}
		case "interface":
			if i+1 < len(tokens) {
				return tokens[i+1], "interface", true
			}
		case "enum":
			if i+1 < len(tokens) {
				return tokens[i+1], "enum", true
			}
		case "@interface":
			if i+1 < len(tokens) {
				return tokens[i+1], "annotation", true
			}
		}
	}
	return "", "", false
}

// matchMethodDecl recognises Java method declarations of the form:
//
//	[modifiers]* returnType methodName ( [params] ) [throws ...] {
func matchMethodDecl(line, parentType string) (*Entity, bool) {
	if parentType == "" {
		return nil, false
	}

	tokens := tokeniseJava(line)
	if len(tokens) < 2 {
		return nil, false
	}

	modifiers := map[string]bool{
		"public": true, "protected": true, "private": true,
		"static": true, "final": true, "abstract": true,
		"synchronized": true, "native": true, "default": true,
		"transient": true, "volatile": true, "strictfp": true,
	}
	reject := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true,
		"return": true, "throw": true, "new": true, "else": true,
		"try": true, "catch": true, "finally": true,
		"import": true, "package": true, "class": true,
		"interface": true, "enum": true,
	}

	// Skip leading modifiers and annotations (tokens starting with @).
	i := 0
	for i < len(tokens) && (modifiers[tokens[i]] || strings.HasPrefix(tokens[i], "@")) {
		i++
	}
	if i >= len(tokens) {
		return nil, false
	}
	if reject[tokens[i]] {
		return nil, false
	}

	// Find the token immediately before '(' — that's the method name.
	retTypeStart := i
	nameIdx := -1
	for j := i; j+1 < len(tokens); j++ {
		if tokens[j+1] == "(" {
			nameIdx = j
			break
		}
	}
	if nameIdx < 0 {
		return nil, false
	}

	name := tokens[nameIdx]
	if name == "" || reject[name] || modifiers[name] {
		return nil, false
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '$' {
			return nil, false
		}
	}

	retType := strings.Join(tokens[retTypeStart:nameIdx], " ")

	// Build signature from the raw line.
	sig := retType + " " + name + "()"
	parenIdx := strings.Index(line, name+"(")
	if parenIdx >= 0 {
		rest := line[parenIdx+len(name):]
		depth := 0
		end := 0
		for j, c := range rest {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					end = j + 1
					break
				}
			}
		}
		if end > 0 {
			sig = retType + " " + name + rest[:end]
		}
	}

	return &Entity{
		Name:      name,
		Type:      "method",
		Kind:      "method",
		Signature: sig,
		Parent:    parentType,
	}, true
}

// --------------------------------------------------------------------------
// Utilities
// --------------------------------------------------------------------------

// tokeniseJava splits a Java source line into identifier/keyword tokens.
// '(' is kept as its own token so matchMethodDecl can find "name (" patterns.
func tokeniseJava(line string) []string {
	var tokens []string
	i := 0
	n := len(line)
	for i < n {
		c := rune(line[i])

		if c == '(' {
			tokens = append(tokens, "(")
			i++
			continue
		}

		// Annotation token: @Foo or @interface
		if c == '@' {
			j := i + 1
			for j < n && (unicode.IsLetter(rune(line[j])) || unicode.IsDigit(rune(line[j])) || line[j] == '_') {
				j++
			}
			tok := line[i:j]
			// @interface is a special keyword in Java for annotation types.
			if j < n && line[j] == ' ' {
				rest := strings.TrimSpace(line[j:])
				if strings.HasPrefix(rest, "interface") {
					tok = "@interface"
					// skip past the word "interface"
					j += 1 + len("interface")
				}
			}
			tokens = append(tokens, tok)
			i = j
			continue
		}

		if unicode.IsLetter(c) || c == '_' || c == '$' {
			j := i
			for j < n && (unicode.IsLetter(rune(line[j])) || unicode.IsDigit(rune(line[j])) || line[j] == '_' || line[j] == '$') {
				j++
			}
			tokens = append(tokens, line[i:j])
			i = j
			continue
		}

		i++
	}
	return tokens
}
