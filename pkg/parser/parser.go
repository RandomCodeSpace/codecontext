package parser

import (
	"path/filepath"
	"strings"
)

type ParseResult struct {
	FilePath     string
	Language     string
	Entities     []*Entity
	Dependencies []*Dependency
}

type Entity struct {
	Name      string
	Type      string // function, class, type, variable, interface
	Kind      string // more specific kind
	Signature string
	StartLine int
	EndLine   int
	Docs      string
}

type Dependency struct {
	Path       string
	Type       string // import, require, from, include
	LineNumber int
}

// Parse routes to the appropriate language parser
func Parse(filePath string, content string) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return parseGo(filePath, content)
	case ".js", ".ts", ".jsx", ".tsx":
		return parseJavaScript(filePath, content)
	case ".py":
		return parsePython(filePath, content)
	default:
		return &ParseResult{
			FilePath:     filePath,
			Language:     "unknown",
			Entities:     []*Entity{},
			Dependencies: []*Dependency{},
		}, nil
	}
}

func parseGo(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Language:     "go",
		Entities:     make([]*Entity, 0),
		Dependencies: make([]*Dependency, 0),
	}

	lines := strings.Split(content, "\n")
	inBlock := false

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Parse imports
		if strings.HasPrefix(trimmed, "import") {
			if strings.HasSuffix(trimmed, "(") {
				inBlock = true
			} else if strings.Contains(trimmed, "\"") {
				// Single line import: import "fmt"
				path := extractImportPath(trimmed)
				if path != "" {
					result.Dependencies = append(result.Dependencies, &Dependency{
						Path:       path,
						Type:       "import",
						LineNumber: lineNum,
					})
				}
			}
		} else if inBlock && strings.HasPrefix(trimmed, ")") {
			inBlock = false
		} else if inBlock && strings.Contains(trimmed, "\"") {
			path := extractImportPath(trimmed)
			if path != "" {
				result.Dependencies = append(result.Dependencies, &Dependency{
					Path:       path,
					Type:       "import",
					LineNumber: lineNum,
				})
			}
		}

		// Parse function definitions: func Name(...) ...
		if strings.HasPrefix(trimmed, "func ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := parts[1]
				// Handle receiver syntax: func (r Receiver) Name(...) ...
				if strings.HasPrefix(name, "(") {
					if len(parts) > 2 {
						name = parts[2]
					}
				}
				// Remove parentheses
				name = strings.Split(name, "(")[0]

				entity := &Entity{
					Name:      name,
					Type:      "function",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse type definitions: type Name ...
		if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := parts[1]
				entity := &Entity{
					Name:      name,
					Type:      "type",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse interface definitions
		if strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, "interface") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := parts[1]
				entity := &Entity{
					Name:      name,
					Type:      "interface",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse const/var declarations
		if strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "var ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := parts[1]
				entityType := "variable"
				if strings.HasPrefix(trimmed, "const") {
					entityType = "constant"
				}
				entity := &Entity{
					Name:      name,
					Type:      entityType,
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}
	}

	return result, nil
}

func parseJavaScript(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Language:     "javascript",
		Entities:     make([]*Entity, 0),
		Dependencies: make([]*Dependency, 0),
	}

	// Check if TypeScript
	if strings.HasSuffix(filePath, ".ts") || strings.HasSuffix(filePath, ".tsx") {
		result.Language = "typescript"
	}

	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Parse imports: import { x } from "module" or import x from "module"
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "import type ") {
			fromIdx := strings.Index(trimmed, "from")
			if fromIdx > -1 {
				rest := trimmed[fromIdx+4:]
				path := extractStringLiteral(rest)
				if path != "" {
					result.Dependencies = append(result.Dependencies, &Dependency{
						Path:       path,
						Type:       "import",
						LineNumber: lineNum,
					})
				}
			}
		}

		// Parse requires: require("module")
		if strings.Contains(trimmed, "require(") {
			path := extractRequirePath(trimmed)
			if path != "" {
				result.Dependencies = append(result.Dependencies, &Dependency{
					Path:       path,
					Type:       "require",
					LineNumber: lineNum,
				})
			}
		}

		// Parse function declarations: function name(...) or const name = (...)
		if strings.HasPrefix(trimmed, "function ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(parts[1], "(")[0]
				entity := &Entity{
					Name:      name,
					Type:      "function",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		} else if strings.Contains(trimmed, "= (") && strings.Contains(trimmed, "=>") {
			// Arrow function: const name = () => ...
			parts := strings.Split(trimmed, "=")
			if len(parts) > 0 {
				name := strings.TrimSpace(parts[0])
				if !strings.Contains(name, " ") && name != "" {
					entity := &Entity{
						Name:      name,
						Type:      "function",
						Signature: trimmed,
						StartLine: lineNum,
						EndLine:   lineNum,
					}
					result.Entities = append(result.Entities, entity)
				}
			}
		}

		// Parse class declarations: class Name ...
		if strings.HasPrefix(trimmed, "class ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(parts[1], "{")[0]
				entity := &Entity{
					Name:      name,
					Type:      "class",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse interface/type definitions (TypeScript)
		if strings.HasPrefix(trimmed, "interface ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(parts[1], "{")[0]
				entity := &Entity{
					Name:      name,
					Type:      "interface",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		} else if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(parts[1], "=")[0]
				name = strings.TrimSpace(name)
				entity := &Entity{
					Name:      name,
					Type:      "type",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse const/let/var declarations
		if strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "let ") || strings.HasPrefix(trimmed, "var ") {
			parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == ' ' || r == '=' })
			if len(parts) > 1 {
				name := parts[1]
				entity := &Entity{
					Name:      name,
					Type:      "variable",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}
	}

	return result, nil
}

func parsePython(filePath string, content string) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Language:     "python",
		Entities:     make([]*Entity, 0),
		Dependencies: make([]*Dependency, 0),
	}

	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Parse imports: import x or from x import y
		if strings.HasPrefix(trimmed, "import ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				module := parts[1]
				result.Dependencies = append(result.Dependencies, &Dependency{
					Path:       module,
					Type:       "import",
					LineNumber: lineNum,
				})
			}
		} else if strings.HasPrefix(trimmed, "from ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				module := parts[1]
				result.Dependencies = append(result.Dependencies, &Dependency{
					Path:       module,
					Type:       "from",
					LineNumber: lineNum,
				})
			}
		}

		// Parse function definitions: def name(...):
		if strings.HasPrefix(trimmed, "def ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(parts[1], "(")[0]
				entity := &Entity{
					Name:      name,
					Type:      "function",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}

		// Parse class definitions: class Name(...):
		if strings.HasPrefix(trimmed, "class ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				name := strings.Split(strings.Split(parts[1], "(")[0], ":")[0]
				entity := &Entity{
					Name:      name,
					Type:      "class",
					Signature: trimmed,
					StartLine: lineNum,
					EndLine:   lineNum,
				}
				result.Entities = append(result.Entities, entity)
			}
		}
	}

	return result, nil
}

// Helper functions

func extractImportPath(line string) string {
	start := strings.Index(line, "\"")
	if start == -1 {
		return ""
	}
	end := strings.Index(line[start+1:], "\"")
	if end == -1 {
		return ""
	}
	return line[start+1 : start+1+end]
}

func extractStringLiteral(s string) string {
	s = strings.TrimSpace(s)
	for _, quote := range []string{"\"", "'", "`"} {
		start := strings.Index(s, quote)
		if start != -1 {
			end := strings.LastIndex(s, quote)
			if end > start {
				return s[start+1 : end]
			}
		}
	}
	return ""
}

func extractRequirePath(line string) string {
	start := strings.Index(line, "(")
	if start == -1 {
		return ""
	}
	end := strings.Index(line[start+1:], ")")
	if end == -1 {
		return ""
	}
	return extractStringLiteral(line[start+1 : start+1+end])
}
