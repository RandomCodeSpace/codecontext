package java

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
	// class/interface/enum declarations
	reClassDecl     = regexp.MustCompile(`^\s*(?:public|protected|private|abstract|final|static)?\s*(?:public|protected|private|abstract|final|static)?\s*class\s+(\w+)`)
	reInterfaceDecl = regexp.MustCompile(`^\s*(?:public|protected|private)?\s*interface\s+(\w+)`)
	reEnumDecl      = regexp.MustCompile(`^\s*(?:public|protected|private)?\s*enum\s+(\w+)`)
	// method declarations: visibility? static? returnType methodName(...)
	reMethodDecl = regexp.MustCompile(`^\s+(?:(?:public|protected|private|static|final|abstract|synchronized|native)\s+)*(\w[\w<>\[\],\s]*)\s+(\w+)\s*(\([^)]*\))\s*(?:throws\s+\w+(?:\s*,\s*\w+)*)?\s*\{`)
	// import statements
	reImport = regexp.MustCompile(`^\s*import\s+(?:static\s+)?([^;]+);`)
)

type JavaParser struct{}

func (p *JavaParser) Parse(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	lines := strings.Split(content, "\n")
	// Track the most recent class/interface context
	classStack := []string{}

	for i, line := range lines {
		lineNum := i + 1

		// Import statements
		if m := reImport.FindStringSubmatch(line); m != nil {
			path := strings.TrimSpace(m[1])
			result.Dependencies = append(result.Dependencies, &Dependency{
				Path:       path,
				Type:       "import",
				LineNumber: lineNum,
				IsLocal:    false,
			})
			continue
		}

		// Class declarations
		if m := reClassDecl.FindStringSubmatch(line); m != nil {
			name := m[1]
			classStack = append(classStack, name)
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      "class",
				Kind:      "class",
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// Interface declarations
		if m := reInterfaceDecl.FindStringSubmatch(line); m != nil {
			name := m[1]
			classStack = append(classStack, name)
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      "interface",
				Kind:      "interface",
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// Enum declarations
		if m := reEnumDecl.FindStringSubmatch(line); m != nil {
			name := m[1]
			classStack = append(classStack, name)
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      "enum",
				Kind:      "enum",
				StartLine: lineNum,
				EndLine:   lineNum,
			})
			continue
		}

		// Method declarations (must be indented, i.e. inside a class)
		if m := reMethodDecl.FindStringSubmatch(line); m != nil {
			returnType := strings.TrimSpace(m[1])
			name := m[2]
			params := m[3]
			// Skip common false positives
			if returnType == "if" || returnType == "for" || returnType == "while" || returnType == "switch" {
				continue
			}
			parent := ""
			if len(classStack) > 0 {
				parent = classStack[len(classStack)-1]
			}
			result.Entities = append(result.Entities, &Entity{
				Name:      name,
				Type:      "method",
				Kind:      "method",
				Signature: returnType + " " + name + params,
				StartLine: lineNum,
				EndLine:   lineNum,
				Parent:    parent,
			})
		}
	}

	return result, nil
}
