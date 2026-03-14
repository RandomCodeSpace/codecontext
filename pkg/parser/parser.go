package parser

import (
	"path/filepath"
	"strings"

	cparser "github.com/RandomCodeSpace/codecontext/pkg/parser/c"
	cppparser "github.com/RandomCodeSpace/codecontext/pkg/parser/cpp"
	goparser "github.com/RandomCodeSpace/codecontext/pkg/parser/go"
	javaparser "github.com/RandomCodeSpace/codecontext/pkg/parser/java"
	jsparser "github.com/RandomCodeSpace/codecontext/pkg/parser/javascript"
	pyparser "github.com/RandomCodeSpace/codecontext/pkg/parser/python"
)

// Parse routes to the appropriate language parser
func Parse(filePath string, content string) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	lang := detectLanguage(ext)

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

	case C:
		parser := &cparser.CParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertCParseResult(filePath, result), nil

	case Cpp:
		parser := &cppparser.CppParser{}
		result, err := parser.Parse(filePath, content)
		if err != nil {
			return nil, err
		}
		return convertCppParseResult(filePath, result), nil

	default:
		return &ParseResult{
			FilePath:     filePath,
			Language:     "unknown",
			Entities:     []*Entity{},
			Dependencies: []*Dependency{},
		}, nil
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

// convertCParseResult converts a C sub-parser result to the main ParseResult.
func convertCParseResult(filePath string, result *cparser.ParseResult) *ParseResult {
	main := &ParseResult{
		FilePath:     filePath,
		Language:     C,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}
	for i, e := range result.Entities {
		main.Entities[i] = &Entity{
			Name: e.Name, Type: e.Type, Kind: e.Kind,
			Signature: e.Signature, StartLine: e.StartLine, EndLine: e.EndLine,
			Docs: e.Docs, Parent: e.Parent, Language: C,
		}
	}
	for i, d := range result.Dependencies {
		main.Dependencies[i] = &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		}
	}
	return main
}

// convertCppParseResult converts a C++ sub-parser result to the main ParseResult.
func convertCppParseResult(filePath string, result *cppparser.ParseResult) *ParseResult {
	main := &ParseResult{
		FilePath:     filePath,
		Language:     Cpp,
		Entities:     make([]*Entity, len(result.Entities)),
		Dependencies: make([]*Dependency, len(result.Dependencies)),
	}
	for i, e := range result.Entities {
		main.Entities[i] = &Entity{
			Name: e.Name, Type: e.Type, Kind: e.Kind,
			Signature: e.Signature, StartLine: e.StartLine, EndLine: e.EndLine,
			Docs: e.Docs, Parent: e.Parent, Language: Cpp,
		}
	}
	for i, d := range result.Dependencies {
		main.Dependencies[i] = &Dependency{
			Path: d.Path, Type: d.Type, LineNumber: d.LineNumber, IsLocal: d.IsLocal,
		}
	}
	return main
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
	case ".c", ".h":
		return C
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx", ".hh":
		return Cpp
	default:
		return ""
	}
}
