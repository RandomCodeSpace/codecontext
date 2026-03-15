package parser

import (
	"strconv"
	"strings"

	goparser "github.com/RandomCodeSpace/codecontext/pkg/parser/go"
	javaparser "github.com/RandomCodeSpace/codecontext/pkg/parser/java"
	jsparser "github.com/RandomCodeSpace/codecontext/pkg/parser/javascript"
	pyparser "github.com/RandomCodeSpace/codecontext/pkg/parser/python"
	sitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Parse routes to the appropriate language parser
func Parse(filePath string, content string) (*ParseResult, error) {
	lang := detectLanguage(filePath)

	switch lang {
	case Go:
		parser := &goparser.GoParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertGoParseResult(result), nil

	case Python:
		parser := &pyparser.PythonParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertPyParseResult(filePath, result), nil

	case JavaScript, TypeScript:
		parser := &jsparser.JSParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertJSParseResult(filePath, lang, result), nil

	case Java:
		parser := &javaparser.JavaParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertJavaParseResult(filePath, result), nil

	default:
		if lang == "" {
			lang = "unknown"
			return &ParseResult{
				FilePath:     filePath,
				Language:     lang,
				Entities:     []*Entity{},
				Dependencies: []*Dependency{},
			}, nil
		}

		return parseGenericWithTags(filePath, content, lang)
	}
}

// convertGoParseResult converts from go parser result to main parser result
func convertGoParseResult(result *goparser.ParseResult) *ParseResult {
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

// convertPyParseResult converts a Python sub-parser result to the main ParseResult.
func convertPyParseResult(filePath string, result *pyparser.ParseResult) *ParseResult {
	main := &ParseResult{
		FilePath:     filePath,
		Language:     Python,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}
	for i, e := range result.Entities {
		main.Entities[i] = &Entity{
			Name: e.Name, Type: e.Type, Kind: e.Kind,
			Signature: e.Signature, StartLine: e.StartLine, EndLine: e.EndLine,
			Docs: e.Docs, Parent: e.Parent, Language: Python,
		}
	}
	for i, d := range result.Dependencies {
		main.Dependencies[i] = &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		}
	}
	return main
}

// convertJSParseResult converts a JavaScript/TypeScript sub-parser result to the main ParseResult.
func convertJSParseResult(filePath string, lang Language, result *jsparser.ParseResult) *ParseResult {
	main := &ParseResult{
		FilePath:     filePath,
		Language:     lang,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}
	for i, e := range result.Entities {
		main.Entities[i] = &Entity{
			Name: e.Name, Type: e.Type, Kind: e.Kind,
			Signature: e.Signature, StartLine: e.StartLine, EndLine: e.EndLine,
			Docs: e.Docs, Parent: e.Parent, Language: lang,
		}
	}
	for i, d := range result.Dependencies {
		main.Dependencies[i] = &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		}
	}
	return main
}

// convertJavaParseResult converts a Java sub-parser result to the main ParseResult.
func convertJavaParseResult(filePath string, result *javaparser.ParseResult) *ParseResult {
	main := &ParseResult{
		FilePath:     filePath,
		Language:     Java,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}
	for i, e := range result.Entities {
		main.Entities[i] = &Entity{
			Name: e.Name, Type: e.Type, Kind: e.Kind,
			Signature: e.Signature, StartLine: e.StartLine, EndLine: e.EndLine,
			Docs: e.Docs, Parent: e.Parent, Language: Java,
		}
	}
	for i, d := range result.Dependencies {
		main.Dependencies[i] = &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		}
	}
	return main
}
func detectLanguage(filePath string) Language {
	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return ""
	}

	// Keep existing parser routing for the languages with custom entity/dependency extraction.
	switch entry.Name {
	case "go":
		return Go
	case "python":
		return Python
	case "javascript":
		return JavaScript
	case "typescript", "tsx":
		return TypeScript
	case "java":
		return Java
	default:
		return Language(entry.Name)
	}
}

func parseGenericWithTags(filePath string, content string, lang Language) (*ParseResult, error) {
	result := &ParseResult{
		FilePath:     filePath,
		Language:     lang,
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		return result, nil
	}

	langDef := entry.Language()
	src := []byte(content)
	p := sitter.NewParser(langDef)

	var err error
	if entry.TokenSourceFactory != nil {
		_, err = p.ParseWithTokenSource(src, entry.TokenSourceFactory(src, langDef))
	} else {
		_, err = p.Parse(src)
	}
	if err != nil {
		// Some grammars may fail on edge inputs; keep indexing resilient.
		return result, nil
	}

	tagsQuery := entry.TagsQuery
	if strings.TrimSpace(tagsQuery) == "" {
		for _, e := range grammars.AllLanguages() {
			if e.Name == entry.Name {
				tagsQuery = e.TagsQuery
				break
			}
		}
	}
	if strings.TrimSpace(tagsQuery) == "" {
		return result, nil
	}

	tagger, err := sitter.NewTagger(langDef, tagsQuery)
	if err != nil {
		return result, nil
	}

	seen := map[string]bool{}
	for _, t := range tagger.Tag(src) {
		if !strings.HasPrefix(t.Kind, "definition.") {
			continue
		}

		entityType := strings.TrimPrefix(t.Kind, "definition.")
		if entityType == "" {
			entityType = "symbol"
		}

		startLine := int(t.Range.StartPoint.Row) + 1
		endLine := int(t.Range.EndPoint.Row) + 1
		if endLine < startLine {
			endLine = startLine
		}

		key := t.Name + "|" + t.Kind + "|" + strconv.Itoa(startLine)
		if seen[key] {
			continue
		}
		seen[key] = true

		result.Entities = append(result.Entities, &Entity{
			Name:        t.Name,
			Type:        entityType,
			Kind:        t.Kind,
			Signature:   t.Name,
			StartLine:   startLine,
			EndLine:     endLine,
			ColumnStart: int(t.NameRange.StartPoint.Column) + 1,
			ColumnEnd:   int(t.NameRange.EndPoint.Column) + 1,
			Language:    lang,
			Scope:       "file",
		})
	}

	return result, nil
}
