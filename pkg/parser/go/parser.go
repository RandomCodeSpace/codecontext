package goparser

import (
	"encoding/json"
	"fmt"
	"go/ast"
	stdparser "go/parser"
	"go/token"
	"strings"
)

// Parse result structures (duplicated to avoid circular import)
type Entity struct {
	Name        string
	Type        string
	Kind        string
	Signature   string
	StartLine   int
	EndLine     int
	ColumnStart int
	ColumnEnd   int
	Docs        string
	Parent      string
	Visibility  string
	Scope       string
	Language    string
	Attributes  map[string]interface{}
}

type Dependency struct {
	Path       string
	Type       string
	LineNumber int
	Resolved   string
	IsLocal    bool
}

type ParseResult struct {
	FilePath     string
	Language     string
	Entities     []*Entity
	Dependencies []*Dependency
}

// GoParser uses Go's stdlib AST parser for accurate parsing
type GoParser struct{}

func (p *GoParser) Language() string {
	return "go"
}

func (p *GoParser) Parse(filePath string, content string) (*ParseResult, error) {
	fset := token.NewFileSet()
	astFile, err := stdparser.ParseFile(fset, filePath, content,
		stdparser.ParseComments|stdparser.DeclarationErrors)
	if err != nil {
		// Don't fail on parse errors, just continue
		fmt.Printf("parse error in %s: %v\n", filePath, err)
	}

	result := &ParseResult{
		FilePath:     filePath,
		Language:     "go",
		Entities:     []*Entity{},
		Dependencies: []*Dependency{},
	}

	if astFile == nil {
		return result, nil
	}

	// Extract imports
	for _, imp := range astFile.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		result.Dependencies = append(result.Dependencies, &Dependency{
			Path:       path,
			Type:       "import",
			LineNumber: fset.Position(imp.Pos()).Line,
			IsLocal:    strings.HasPrefix(path, "."),
		})
	}

	// Extract declarations (functions, types, interfaces, etc.)
	for _, decl := range astFile.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p.extractFunction(d, fset, result)
		case *ast.GenDecl:
			p.extractGenDecl(d, fset, result)
		}
	}

	return result, nil
}

func (p *GoParser) extractFunction(funcDecl *ast.FuncDecl, fset *token.FileSet, result *ParseResult) {
	name := funcDecl.Name.Name
	startPos := fset.Position(funcDecl.Pos())
	endPos := fset.Position(funcDecl.End())

	entityType := "function"
	visibility := "public"
	scope := "global"
	parent := ""

	// Check if it's a method (has receiver)
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		entityType = "method"
		receiver := funcDecl.Recv.List[0]

		// Extract receiver type
		if ident, ok := receiver.Type.(*ast.Ident); ok {
			parent = ident.Name
		} else if starExpr, ok := receiver.Type.(*ast.StarExpr); ok {
			if ident, ok := starExpr.X.(*ast.Ident); ok {
				parent = ident.Name
			}
		}
		scope = "class"
	}

	// Check visibility (unexported functions start with lowercase)
	if strings.ToLower(name[0:1]) == name[0:1] {
		visibility = "private"
	}

	signature := p.buildFunctionSignature(funcDecl, fset)

	entity := &Entity{
		Name:        name,
		Type:        entityType,
		Signature:   signature,
		StartLine:   startPos.Line,
		EndLine:     endPos.Line,
		ColumnStart: startPos.Column,
		ColumnEnd:   endPos.Column,
		Parent:      parent,
		Visibility:  visibility,
		Scope:       scope,
		Language:    "go",
	}

	// Extract documentation from comments
	if funcDecl.Doc != nil {
		entity.Docs = funcDecl.Doc.Text()
	}

	result.Entities = append(result.Entities, entity)
}

func (p *GoParser) extractGenDecl(genDecl *ast.GenDecl, fset *token.FileSet, result *ParseResult) {
	for _, spec := range genDecl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			p.extractTypeSpec(s, genDecl, fset, result)
		case *ast.ValueSpec:
			p.extractValueSpec(s, genDecl, fset, result)
		}
	}
}

func (p *GoParser) extractTypeSpec(typeSpec *ast.TypeSpec, genDecl *ast.GenDecl, fset *token.FileSet, result *ParseResult) {
	name := typeSpec.Name.Name
	startPos := fset.Position(typeSpec.Pos())
	endPos := fset.Position(typeSpec.End())

	entityType := "type"
	visibility := "public"

	if strings.ToLower(name[0:1]) == name[0:1] {
		visibility = "private"
	}

	// Determine if it's an interface or struct
	if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		entityType = "interface"
	} else if _, ok := typeSpec.Type.(*ast.StructType); ok {
		entityType = "struct"
	}

	entity := &Entity{
		Name:        name,
		Type:        entityType,
		StartLine:   startPos.Line,
		EndLine:     endPos.Line,
		ColumnStart: startPos.Column,
		ColumnEnd:   endPos.Column,
		Visibility:  visibility,
		Scope:       "global",
		Language:    "go",
	}

	// Add documentation
	if genDecl.Doc != nil {
		entity.Docs = genDecl.Doc.Text()
	}

	result.Entities = append(result.Entities, entity)

	// Extract methods for structs and interfaces
	p.extractStructMembers(name, typeSpec.Type, fset, result)
}

func (p *GoParser) extractStructMembers(typeName string, typeNode ast.Expr, fset *token.FileSet, result *ParseResult) {
	switch t := typeNode.(type) {
	case *ast.StructType:
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				if field.Names == nil {
					continue // Embedded type
				}
				for _, name := range field.Names {
					if name.IsExported() {
						entity := &Entity{
							Name:        name.Name,
							Type:        "field",
							Parent:      typeName,
							Visibility:  "public",
							Scope:       "class",
							Language:    "go",
							StartLine:   fset.Position(field.Pos()).Line,
							ColumnStart: fset.Position(field.Pos()).Column,
						}
						result.Entities = append(result.Entities, entity)
					}
				}
			}
		}

	case *ast.InterfaceType:
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if method.Names == nil {
					continue // Embedded interface
				}
				for _, name := range method.Names {
					entity := &Entity{
						Name:       name.Name,
						Type:       "method",
						Kind:       "interface_method",
						Parent:     typeName,
						Visibility: "public",
						Scope:      "class",
						Language:   "go",
						StartLine:  fset.Position(method.Pos()).Line,
					}
					result.Entities = append(result.Entities, entity)
				}
			}
		}
	}
}

func (p *GoParser) extractValueSpec(valueSpec *ast.ValueSpec, genDecl *ast.GenDecl, fset *token.FileSet, result *ParseResult) {
	entityType := "variable"
	if genDecl.Tok == token.CONST {
		entityType = "constant"
	}

	for _, name := range valueSpec.Names {
		visibility := "public"
		if strings.ToLower(name.Name[0:1]) == name.Name[0:1] {
			visibility = "private"
		}

		entity := &Entity{
			Name:        name.Name,
			Type:        entityType,
			StartLine:   fset.Position(name.Pos()).Line,
			ColumnStart: fset.Position(name.Pos()).Column,
			Visibility:  visibility,
			Scope:       "global",
			Language:    "go",
		}

		if genDecl.Doc != nil {
			entity.Docs = genDecl.Doc.Text()
		}

		result.Entities = append(result.Entities, entity)
	}
}

func (p *GoParser) buildFunctionSignature(funcDecl *ast.FuncDecl, fset *token.FileSet) string {
	var sb strings.Builder

	sb.WriteString("func ")

	// Add receiver if method
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
		sb.WriteString("(")
		p.writeFieldList(funcDecl.Recv, &sb)
		sb.WriteString(") ")
	}

	// Function name
	sb.WriteString(funcDecl.Name.Name)

	// Parameters
	sb.WriteString("(")
	p.writeFieldList(funcDecl.Type.Params, &sb)
	sb.WriteString(")")

	// Return types
	if funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0 {
		sb.WriteString(" (")
		p.writeFieldList(funcDecl.Type.Results, &sb)
		sb.WriteString(")")
	}

	return sb.String()
}

func (p *GoParser) writeFieldList(fieldList *ast.FieldList, sb *strings.Builder) {
	if fieldList == nil {
		return
	}

	for i, field := range fieldList.List {
		if i > 0 {
			sb.WriteString(", ")
		}

		// Names
		for j, name := range field.Names {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name.Name)
		}

		if len(field.Names) > 0 {
			sb.WriteString(" ")
		}

		// Type
		p.writeExpr(field.Type, sb)
	}
}

func (p *GoParser) writeExpr(expr ast.Expr, sb *strings.Builder) {
	switch e := expr.(type) {
	case *ast.Ident:
		sb.WriteString(e.Name)
	case *ast.StarExpr:
		sb.WriteString("*")
		p.writeExpr(e.X, sb)
	case *ast.SelectorExpr:
		p.writeExpr(e.X, sb)
		sb.WriteString(".")
		sb.WriteString(e.Sel.Name)
	case *ast.ArrayType:
		sb.WriteString("[]")
		p.writeExpr(e.Elt, sb)
	case *ast.MapType:
		sb.WriteString("map[")
		p.writeExpr(e.Key, sb)
		sb.WriteString("]")
		p.writeExpr(e.Value, sb)
	case *ast.InterfaceType:
		sb.WriteString("interface{}")
	default:
		sb.WriteString("interface{}")
	}
}

// Helper to encode attributes as JSON
func encodeAttributes(attrs map[string]interface{}) string {
	data, _ := json.Marshal(attrs)
	return string(data)
}
