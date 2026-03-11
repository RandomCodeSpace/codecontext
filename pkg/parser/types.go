package parser

// Language represents supported programming languages
type Language string

const (
	Go         Language = "go"
	Python     Language = "python"
	JavaScript Language = "javascript"
	TypeScript Language = "typescript"
	Java       Language = "java"
)

type ParseResult struct {
	FilePath     string
	Language     Language
	Entities     []*Entity
	Dependencies []*Dependency
}

type Entity struct {
	Name        string
	Type        string            // function, class, type, interface, method, etc.
	Kind        string            // specific: async_function, arrow_function, etc.
	Signature   string
	StartLine   int
	EndLine     int
	ColumnStart int
	ColumnEnd   int
	Docs        string            // Documentation/comments
	Parent      string            // Parent entity name for nested entities
	Visibility  string            // public, private, protected, internal
	Scope       string            // global, class, file, module, local
	Language    Language          // go, python, javascript, java
	Attributes  map[string]interface{} // Language-specific metadata
}

type Dependency struct {
	Path       string
	Type       string // import, require, from, include
	LineNumber int
	Resolved   string // Resolved path if determined
	IsLocal    bool   // True if local import
}

// LanguageParser defines the interface for language-specific parsers
type LanguageParser interface {
	Language() Language
	Parse(filePath string, content string) (*ParseResult, error)
}
