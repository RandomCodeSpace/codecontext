package javascript

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
	// Functions: function foo(...), async function foo(...)
	reFuncDecl = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)\s*(\([^)]*\))`)
	// Arrow / const functions: const foo = (...) => / const foo = async (...) =>
	reArrowFunc = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(?[^)]*\)?\s*=>`)
	// Class declarations
	reClassDecl = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?class\s+(\w+)`)
	// Method inside class body: methodName(...) {  or async methodName(...)
	reMethod = regexp.MustCompile(`^\s+(?:async\s+)?(?:static\s+)?(?:get\s+|set\s+)?(\w+)\s*(\([^)]*\))\s*\{`)
	// ES6 imports: import ... from 'module'
	reImportFrom = regexp.MustCompile(`^\s*import\s+.*from\s+['"]([^'"]+)['"]`)
	// require: require('module')
	reRequire = regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
)

type JSParser struct{}

func (p *JSParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	lines := strings.Split(content, "\n")
	inClass := false
	className := ""
	classIndent := 0

	for i, line := range lines {
		lineNum := i + 1

		// Track class context
		if m := reClassDecl.FindStringSubmatch(line); m != nil {
			inClass = true
			className = m[1]
			classIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			result.Entities = append(result.Entities, &Entity{
				Name:      className,
				Type:      "class",
				Kind:      "class",
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// Exit class context when we see closing brace at same indent
		if inClass && strings.TrimSpace(line) == "}" {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= classIndent {
				inClass = false
				className = ""
			}
		}

		// Methods inside class
		if inClass {
			if m := reMethod.FindStringSubmatch(line); m != nil {
				name := m[1]
				if name != "if" && name != "for" && name != "while" && name != "switch" {
					kind := "method"
					if strings.Contains(line, "async ") {
						kind = "async_method"
					}
					result.Entities = append(result.Entities, &Entity{
						Name:      name,
						Type:      "method",
						Kind:      kind,
						Signature: name + m[2],
						StartLine: lineNum,
						EndLine:   lineNum,
						Parent:    className,
					})
					continue
				}
			}
		}

		// Top-level function declarations
		if m := reFuncDecl.FindStringSubmatch(line); m != nil {
			kind := "function"
			if strings.Contains(line, "async ") {
				kind = "async_function"
			}
			result.Entities = append(result.Entities, &Entity{
				Name:      m[1],
				Type:      "function",
				Kind:      kind,
				Signature: m[1] + m[2],
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// Arrow functions
		if m := reArrowFunc.FindStringSubmatch(line); m != nil {
			kind := "arrow_function"
			if strings.Contains(line, "async ") {
				kind = "async_arrow_function"
			}
			result.Entities = append(result.Entities, &Entity{
				Name:      m[1],
				Type:      "function",
				Kind:      kind,
				Signature: m[1],
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// ES6 imports
		if m := reImportFrom.FindStringSubmatch(line); m != nil {
			path := m[1]
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "import",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
			continue
		}

		// require()
		for _, m := range reRequire.FindAllStringSubmatch(line, -1) {
			path := m[1]
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "require",
				LineNumber: lineNum,
				IsLocal:    strings.HasPrefix(path, "."),
			})
		}
	}

	return result, nil
}
