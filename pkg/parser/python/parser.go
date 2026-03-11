package python

import (
	"regexp"
	"strings"
)

// ParseResult mirrors the main parser types to avoid circular imports
type ParseResult struct {
	FilePath     string
	Entities     []*Entity
	Dependencies []*Dependency
}

type Entity struct {
	Name      string
	Type      string
	Kind      string
	Signature string
	StartLine int
	EndLine   int
	Docs      string
	Parent    string
}

type Dependency struct {
	Path       string
	Type       string
	LineNumber int
	IsLocal    bool
}

var (
	reFuncDef    = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+(\w+)\s*(\([^)]*\))`)
	reClassDef   = regexp.MustCompile(`^(\s*)class\s+(\w+)`)
	reImport     = regexp.MustCompile(`^\s*import\s+(.+)`)
	reFromImport = regexp.MustCompile(`^\s*from\s+(\S+)\s+import\s+(.+)`)
	reDocString  = regexp.MustCompile(`^\s*"""`)
)

type PythonParser struct{}

func (p *PythonParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	lines := strings.Split(content, "\n")
	classStack := []string{}  // track current class context

	for i, line := range lines {
		lineNum := i + 1

		// Detect class definitions
		if m := reClassDef.FindStringSubmatch(line); m != nil {
			indent := len(m[1])
			// pop classes from stack that are at same or deeper indent
			for len(classStack) > 0 {
				last := classStack[len(classStack)-1]
				_ = last
				classStack = classStack[:len(classStack)-1]
			}
			_ = indent
			className := m[2]
			classStack = append(classStack, className)
			entity := &Entity{
				Name:      className,
				Type:      "class",
				Kind:      "class",
				StartLine: lineNum,
				EndLine:   lineNum,
			}
			// Check for docstring on next non-empty line
			if i+1 < len(lines) && reDocString.MatchString(lines[i+1]) {
				entity.Docs = extractDocString(lines, i+1)
			}
			result.Entities = append(result.Entities, entity)
			continue
		}

		// Detect function/method definitions
		if m := reFuncDef.FindStringSubmatch(line); m != nil {
			name := m[2]
			sig := m[2] + m[3]
			entityType := "function"
			parent := ""
			if len(classStack) > 0 {
				entityType = "method"
				parent = classStack[len(classStack)-1]
			}
			kind := "function"
			if strings.Contains(line, "async def") {
				kind = "async_function"
			}
			entity := &Entity{
				Name:      name,
				Type:      entityType,
				Kind:      kind,
				Signature: sig,
				StartLine: lineNum,
				EndLine:   lineNum,
				Parent:    parent,
			}
			if i+1 < len(lines) && reDocString.MatchString(lines[i+1]) {
				entity.Docs = extractDocString(lines, i+1)
			}
			result.Entities = append(result.Entities, entity)
			continue
		}

		// Detect imports
		if m := reFromImport.FindStringSubmatch(line); m != nil {
			path := m[1]
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "from",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
			continue
		}
		if m := reImport.FindStringSubmatch(line); m != nil {
			for _, mod := range strings.Split(m[1], ",") {
				mod = strings.TrimSpace(mod)
				// handle "import x as y"
				if idx := strings.Index(mod, " as "); idx >= 0 {
					mod = strings.TrimSpace(mod[:idx])
				}
				if mod != "" {
					result.Dependencies = append(result.Dependencies, &Dependency{
						Path:       mod,
						Type:       "import",
						LineNumber: lineNum,
						IsLocal:    false,
					})
				}
			}
		}
	}

	return result, nil
}

func extractDocString(lines []string, startIdx int) string {
	line := strings.TrimSpace(lines[startIdx])
	// single-line docstring
	if strings.HasPrefix(line, `"""`) && strings.HasSuffix(line, `"""`) && len(line) > 6 {
		return strings.Trim(line, `"`)
	}
	// multi-line: collect until closing """
	var parts []string
	parts = append(parts, strings.TrimPrefix(line, `"""`))
	for i := startIdx + 1; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if strings.HasSuffix(l, `"""`) {
			parts = append(parts, strings.TrimSuffix(l, `"""`))
			break
		}
		parts = append(parts, l)
	}
	return strings.Join(parts, " ")
}
