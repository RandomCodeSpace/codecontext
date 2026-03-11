// Package python implements an indentation-aware AST parser for Python source files.
//
// Unlike a simple regex scanner, this parser:
//   - Tracks triple-quoted string state across lines so keywords inside strings
//     are never misidentified as declarations.
//   - Uses Python's indentation rules to compute accurate EndLine for every entity.
//   - Builds correct parent/child relationships (methods belong to their class).
//   - Extracts inline docstrings for functions and classes.
package python

import (
	"strings"
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
	Signature string // e.g. "greet(name, greeting='Hello')"
	StartLine int
	EndLine   int
	Docs      string // first docstring, if any
	Parent    string // name of enclosing class, empty for top-level
}

// Dependency represents an import statement.
type Dependency struct {
	Path       string
	Type       string // "import" | "from"
	LineNumber int
	IsLocal    bool // true when path starts with "."
}

// --------------------------------------------------------------------------
// Tokeniser helpers
// --------------------------------------------------------------------------

// tripleState tracks whether we are currently inside a triple-quoted string.
type tripleState struct {
	active bool   // are we inside a triple string?
	quote  string // `"""` or `'''`
}

// stripLine returns the structural content of a Python source line — all
// string literals and inline comments are replaced with spaces so their byte
// length is preserved (keeping column positions meaningful) but their content
// cannot trigger false keyword matches.
//
// state is updated in-place across calls so multi-line triple-quoted strings
// are handled correctly.
func stripLine(raw string, ts *tripleState) string {
	buf := []byte(raw)
	i := 0

	if ts.active {
		// We are inside a triple string from a previous line.
		end := strings.Index(raw, ts.quote)
		if end < 0 {
			// Whole line is part of the triple string.
			return ""
		}
		// Clear up to and including the closing delimiter.
		for k := 0; k <= end+2; k++ {
			if k < len(buf) {
				buf[k] = ' '
			}
		}
		ts.active = false
		i = end + 3
	}

	for i < len(raw) {
		c := raw[i]

		// Single-line comment.
		if c == '#' {
			for k := i; k < len(buf); k++ {
				buf[k] = ' '
			}
			break
		}

		// Triple-quoted strings (must check before single-quoted).
		if i+2 < len(raw) {
			if raw[i] == '"' && raw[i+1] == '"' && raw[i+2] == '"' {
				ts.quote = `"""`
				end := strings.Index(raw[i+3:], `"""`)
				if end >= 0 {
					// Whole triple string on this line.
					closeAt := i + 3 + end + 3
					for k := i; k < closeAt && k < len(buf); k++ {
						buf[k] = ' '
					}
					i = closeAt
					continue
				}
				// Extends to next line(s).
				for k := i; k < len(buf); k++ {
					buf[k] = ' '
				}
				ts.active = true
				break
			}
			if raw[i] == '\'' && raw[i+1] == '\'' && raw[i+2] == '\'' {
				ts.quote = `'''`
				end := strings.Index(raw[i+3:], `'''`)
				if end >= 0 {
					closeAt := i + 3 + end + 3
					for k := i; k < closeAt && k < len(buf); k++ {
						buf[k] = ' '
					}
					i = closeAt
					continue
				}
				for k := i; k < len(buf); k++ {
					buf[k] = ' '
				}
				ts.active = true
				break
			}
		}

		// Single-quoted strings.
		if c == '"' || c == '\'' {
			q := c
			buf[i] = ' '
			i++
			for i < len(raw) {
				if raw[i] == '\\' {
					buf[i] = ' '
					i++
					if i < len(raw) {
						buf[i] = ' '
						i++
					}
					continue
				}
				if raw[i] == q {
					buf[i] = ' '
					i++
					break
				}
				buf[i] = ' '
				i++
			}
			continue
		}

		i++
	}

	return string(buf)
}

// indentOf returns the indentation level of raw (tabs counted as 4 spaces).
func indentOf(raw string) int {
	n := 0
	for _, c := range raw {
		switch c {
		case ' ':
			n++
		case '\t':
			n = ((n / 4) + 1) * 4
		default:
			return n
		}
	}
	return n
}

// --------------------------------------------------------------------------
// Scope tracking
// --------------------------------------------------------------------------

type scopeKind int

const (
	scopeClass scopeKind = iota
	scopeFunc
)

type scopeEntry struct {
	kind      scopeKind
	name      string
	indent    int // indentation of the def/class keyword line
	entityIdx int // index into result.Entities for later EndLine update
}

// currentClass returns the name of the innermost enclosing class, or "".
func currentClass(stack []scopeEntry) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].kind == scopeClass {
			return stack[i].name
		}
	}
	return ""
}

// --------------------------------------------------------------------------
// Signature extraction
// --------------------------------------------------------------------------

// extractFuncDef parses a "def name(...) -> ret:" line (after leading
// whitespace/async have already been removed) and returns the function name
// and a compact signature string.  Returns ("", "") on failure.
func extractFuncDef(line string) (name, sig string) {
	rest := strings.TrimPrefix(line, "def ")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}

	// Name ends at the first '('.
	parenIdx := strings.IndexByte(rest, '(')
	if parenIdx <= 0 {
		return "", ""
	}
	name = strings.TrimSpace(rest[:parenIdx])
	if name == "" {
		return "", ""
	}

	// Find the matching closing ')' for the parameter list.
	depth := 0
	closeAt := -1
	for i := parenIdx; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeAt = i
			}
		}
		if closeAt >= 0 {
			break
		}
	}
	if closeAt < 0 {
		closeAt = len(rest) - 1
	}
	sig = name + rest[parenIdx:closeAt+1]
	return name, sig
}

// extractClassName returns the class name from a "class Name..." line.
func extractClassName(line string) string {
	rest := strings.TrimPrefix(line, "class ")
	rest = strings.TrimSpace(rest)
	for i, c := range rest {
		if c == '(' || c == ':' || c == ' ' || c == '\t' {
			return rest[:i]
		}
	}
	return strings.TrimSuffix(rest, ":")
}

// extractDocstring returns the first docstring from lines starting at idx
// (zero-based), handling both single-line and multi-line triple-quoted forms.
func extractDocstring(lines []string, startIdx int) string {
	if startIdx >= len(lines) {
		return ""
	}
	trimmed := strings.TrimSpace(lines[startIdx])

	hasDouble := strings.HasPrefix(trimmed, `"""`)
	hasSingle := strings.HasPrefix(trimmed, `'''`)
	if !hasDouble && !hasSingle {
		return ""
	}

	delim := `"""`
	if hasSingle {
		delim = `'''`
	}

	inner := trimmed[3:]
	// Single-line: ends on the same line.
	if idx := strings.Index(inner, delim); idx >= 0 {
		return strings.TrimSpace(inner[:idx])
	}

	// Multi-line: collect until we find the closing delimiter.
	var parts []string
	parts = append(parts, strings.TrimSpace(inner))
	for i := startIdx + 1; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if idx := strings.Index(l, delim); idx >= 0 {
			if idx > 0 {
				parts = append(parts, strings.TrimSpace(l[:idx]))
			}
			break
		}
		parts = append(parts, l)
	}
	return strings.Join(parts, " ")
}

// --------------------------------------------------------------------------
// Main parser
// --------------------------------------------------------------------------

// PythonParser is the entry point for Python source files.
type PythonParser struct{}

// Parse parses a Python source file and returns all entities and dependencies.
func (p *PythonParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Scope stack: tracks open def/class blocks.
	var stack []scopeEntry

	// Triple-string tokeniser state.
	ts := &tripleState{}

	// closeScopes pops all scope entries whose indent >= current indent,
	// setting their EndLine to lastNonEmpty.
	closeScopes := func(currentIndent, lastNonEmpty int) {
		for len(stack) > 0 && stack[len(stack)-1].indent >= currentIndent {
			e := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			end := lastNonEmpty
			if end < result.Entities[e.entityIdx].StartLine {
				end = result.Entities[e.entityIdx].StartLine
			}
			result.Entities[e.entityIdx].EndLine = end
		}
	}

	lastNonEmpty := 0 // line number of the last non-blank, non-comment line

	for lineIdx, rawLine := range lines {
		lineNum := lineIdx + 1

		stripped := stripLine(rawLine, ts)
		trimmed := strings.TrimSpace(stripped)

		if trimmed == "" {
			continue
		}

		lastNonEmpty = lineNum
		indent := indentOf(rawLine)

		// Close any scopes that this line outdents past.
		closeScopes(indent, lineNum-1)

		// ---- from X import Y ----
		if strings.HasPrefix(trimmed, "from ") && strings.Contains(trimmed, " import ") {
			idx := strings.Index(trimmed, " import ")
			path := strings.TrimSpace(trimmed[5:idx])
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "from",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
			continue
		}

		// ---- import X [as Y] [, Z [as W]] ----
		if strings.HasPrefix(trimmed, "import ") {
			rest := strings.TrimPrefix(trimmed, "import ")
			for _, mod := range strings.Split(rest, ",") {
				mod = strings.TrimSpace(mod)
				if idx := strings.Index(mod, " as "); idx >= 0 {
					mod = strings.TrimSpace(mod[:idx])
				}
				if mod != "" {
					result.Dependencies = append(result.Dependencies, &Dependency{
						Path:       mod,
						Type:       "import",
						LineNumber: lineNum,
					})
				}
			}
			continue
		}

		// ---- class Name[(Base)]: ----
		if strings.HasPrefix(trimmed, "class ") {
			name := extractClassName(trimmed)
			if name != "" {
				parent := currentClass(stack)
				entityIdx := len(result.Entities)
				ent := &Entity{
					Name:      name,
					Type:      "class",
					Kind:      "class",
					StartLine: lineNum,
					EndLine:   lineNum,
					Parent:    parent,
				}
				// Docstring is on the next non-blank line inside the body.
				ent.Docs = extractDocstring(lines, lineIdx+1)
				result.Entities = append(result.Entities, ent)
				stack = append(stack, scopeEntry{
					kind:      scopeClass,
					name:      name,
					indent:    indent,
					entityIdx: entityIdx,
				})
			}
			continue
		}

		// ---- [async] def name(params): ----
		isAsync := false
		defLine := trimmed
		if strings.HasPrefix(trimmed, "async ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "async "))
			if strings.HasPrefix(rest, "def ") {
				isAsync = true
				defLine = rest
			}
		}

		if strings.HasPrefix(defLine, "def ") {
			name, sig := extractFuncDef(defLine)
			if name != "" {
				parent := currentClass(stack)
				entityType := "function"
				kind := "function"
				if parent != "" {
					entityType = "method"
					kind = "method"
				}
				if isAsync {
					kind = "async_" + kind
				}
				entityIdx := len(result.Entities)
				ent := &Entity{
					Name:      name,
					Type:      entityType,
					Kind:      kind,
					Signature: sig,
					StartLine: lineNum,
					EndLine:   lineNum,
					Parent:    parent,
				}
				ent.Docs = extractDocstring(lines, lineIdx+1)
				result.Entities = append(result.Entities, ent)
				stack = append(stack, scopeEntry{
					kind:      scopeFunc,
					name:      name,
					indent:    indent,
					entityIdx: entityIdx,
				})
			}
		}
	}

	// Close all remaining open scopes at EOF.
	for _, e := range stack {
		end := lastNonEmpty
		if end < result.Entities[e.entityIdx].StartLine {
			end = result.Entities[e.entityIdx].StartLine
		}
		result.Entities[e.entityIdx].EndLine = totalLines
		_ = end
	}

	return result, nil
}
