package parser

import (
	"path/filepath"
	"strings"

	goparser "github.com/RandomCodeSpace/codecontext/pkg/parser/go"
)

// Parse routes to the appropriate language parser
func Parse(filePath string, content string) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	lang := detectLanguage(ext)

	switch lang {
	case Go:
		// Use the Go AST parser
		parser := &goparser.GoParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		// Convert from go parser types to main types
		return convertParseResult(result), nil

	case Python, JavaScript, TypeScript, Java:
		// Fallback to empty for now - proper parsers coming soon
		return &ParseResult{
			FilePath:     filePath,
			Language:     lang,
			Entities:     []*Entity{},
			Dependencies: []*Dependency{},
		}, nil

	default:
		return &ParseResult{
			FilePath:     filePath,
			Language:     "unknown",
			Entities:     []*Entity{},
			Dependencies: []*Dependency{},
		}, nil
	}
}

// convertParseResult converts from go parser result to main parser result
func convertParseResult(result *goparser.ParseResult) *ParseResult {
	mainResult := &ParseResult{
		FilePath:     result.FilePath,
		Language:     Go,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}

	for i, e := range result.Entities {
		mainResult.Entities[i] = &Entity{
			Name:        e.Name,
			Type:        e.Type,
			Kind:        e.Kind,
			Signature:   e.Signature,
			StartLine:   e.StartLine,
			EndLine:     e.EndLine,
			ColumnStart: e.ColumnStart,
			ColumnEnd:   e.ColumnEnd,
			Docs:        e.Docs,
			Parent:      e.Parent,
			Visibility:  e.Visibility,
			Scope:       e.Scope,
			Language:    Go,
			Attributes:  e.Attributes,
		}
	}

	for i, d := range result.Dependencies {
		mainResult.Dependencies[i] = &Dependency{
			Path:       d.Path,
			Type:       d.Type,
			LineNumber: d.LineNumber,
			Resolved:   d.Resolved,
			IsLocal:    d.IsLocal,
		}
	}

	return mainResult
}

func detectLanguage(ext string) Language {
	switch ext {
	case ".go":
		return Go
	case ".py":
		return Python
	case ".js", ".jsx":
		return JavaScript
	case ".ts", ".tsx":
		return TypeScript
	case ".java":
		return Java
	default:
		return ""
	}
}
