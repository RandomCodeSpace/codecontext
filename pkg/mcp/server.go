package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
)

// Server wraps the official MCP SDK server with codecontext-specific tools.
type Server struct {
	inner   *mcpsdk.Server
	indexer *indexer.Indexer
	logger  *slog.Logger
}

// New creates a new MCP server with all codecontext tools registered.
func New(idx *indexer.Indexer, version string) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	inner := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "codecontext",
		Version: version,
	}, &mcpsdk.ServerOptions{
		Logger: logger,
	})

	s := &Server{
		inner:   inner,
		indexer: idx,
		logger:  logger,
	}

	s.registerTools()
	return s
}

// SetLogger replaces the default logger.
func (s *Server) SetLogger(l *slog.Logger) { s.logger = l }

// Inner returns the underlying MCP SDK server for direct transport use.
func (s *Server) Inner() *mcpsdk.Server { return s.inner }

// --------------------------------------------------------------------------
// Tool registration
// --------------------------------------------------------------------------

type indexDirArgs struct {
	Path string `json:"path" jsonschema:"Path to the directory to index,required"`
}

type queryEntityArgs struct {
	Name string `json:"name" jsonschema:"Name of the entity to search for,required"`
}

type queryCallGraphArgs struct {
	EntityID float64 `json:"entity_id" jsonschema:"ID of the entity to get call graph for,required"`
}

type queryDepsArgs struct {
	Path string `json:"path" jsonschema:"Path of the file to query,required"`
}

type graphStatsArgs struct{}

type getDocsArgs struct {
	Name       string `json:"name,omitempty" jsonschema:"Entity name (scope=entity) or file path (scope=file). Omit for scope=project."`
	Scope      string `json:"scope,omitempty" jsonschema:"entity: single named symbol; file: all symbols in one file; project: whole-codebase overview. Defaults to entity."`
	FormatHint string `json:"format_hint,omitempty" jsonschema:"Optional hint for the agent reformatting this output."`
}

type listFilesArgs struct {
	Language string `json:"language,omitempty" jsonschema:"Optional language extension filter (e.g. '.go', '.ts')"`
}

type getFileOutlineArgs struct {
	Path string `json:"path" jsonschema:"Path of the file to outline,required"`
}

type findUsagesArgs struct {
	EntityID float64 `json:"entity_id" jsonschema:"ID of the entity to find usages for,required"`
}

type searchEntitiesArgs struct {
	Query string `json:"query" jsonschema:"Substring search query,required"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results to return (default: 20)"`
}

type getEntityCodeArgs struct {
	EntityID float64 `json:"entity_id" jsonschema:"ID of the entity to get source code for,required"`
}

type getFileImportsArgs struct {
	Path string `json:"path" jsonschema:"Path of the file to get imports for,required"`
}

func (s *Server) registerTools() {
	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "index_directory",
		Description: "Parses and indexes all source code in a directory tree into the local graph database. Run this first or when code changes heavily to ensure the graph DB is up to date.",
	}, s.handleIndexDirectory)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_entity",
		Description: "Queries the code graph for a specific entity by precise name (e.g., 'MyClass', 'myFunction'). Returns structural metadata such as type, file location, and line ranges. Useful for pinpointing where an entity is defined. (Requires EXACT match; use search_entities for substring).",
	}, s.handleQueryEntity)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_call_graph",
		Description: "Retrieves outbound calls made BY a specific entity. Pass the entity ID to discover what other functions/methods this entity interacts with (children). Useful for understanding an entity's internal behavior.",
	}, s.handleQueryCallGraph)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_dependencies",
		Description: "Retrieves outbound dependencies (imports, requires) for a specific file. Useful for mapping file-level interactions and understanding module coupling.",
	}, s.handleQueryDependencies)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "graph_stats",
		Description: "Returns high-level graph statistics like the total number of files, entities, dependencies, lines of code, and tokens indexed in the project.",
	}, s.handleGraphStats)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name: "get_docs",
		Description: "Returns highly detailed structured documentation for an entity or file. Note: VERY TOKEN HEAVY. Only use this when deep analysis is needed (e.g., requires reading full docs, parameters, and signatures). For structural overviews, prefer get_file_outline or query_entity.",
	}, s.handleGetDocs)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "list_files",
		Description: "Lists all source files managed by the indexer. Returns simple paths and languages. Useful for discovering the codebase layout without running bash ls/find commands.",
	}, s.handleListFiles)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "get_file_outline",
		Description: "Provides a lightweight structural overview of a file. Returns a summary of entities (classes, methods, functions) within the file with their line ranges, skipping heavy signatures and docs to save tokens.",
	}, s.handleGetFileOutline)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "find_usages",
		Description: "Finds inbound references to a specific entity (Reverse Call Graph). Pass the entity ID to discover what other functions/methods call it (parents/callers).",
	}, s.handleFindUsages)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "search_entities",
		Description: "Substring search for entities across the entire codebase. Helpful when you only know part of a class/function name. Returns a summarized list of matches with file locations.",
	}, s.handleSearchEntities)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "get_entity_code",
		Description: "Extracts and returns the exact source code snippet for a specific entity ID. Much more token-efficient than reading the entire file.",
	}, s.handleGetEntityCode)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "get_file_imports",
		Description: "Lists the raw import paths defined in a single file to show its dependency footprint in just a few tokens.",
	}, s.handleGetFileImports)
}

// --------------------------------------------------------------------------
// Tool handlers
// --------------------------------------------------------------------------

func (s *Server) handleIndexDirectory(_ context.Context, _ *mcpsdk.CallToolRequest, args indexDirArgs) (*mcpsdk.CallToolResult, any, error) {
	if err := s.indexer.IndexDirectory(args.Path); err != nil {
		return nil, nil, err
	}
	return textResult(fmt.Sprintf("Successfully indexed directory: %s", args.Path)), nil, nil
}

func (s *Server) handleQueryEntity(_ context.Context, _ *mcpsdk.CallToolRequest, args queryEntityArgs) (*mcpsdk.CallToolResult, any, error) {
	entities, err := s.indexer.QueryEntity(args.Name)
	if err != nil {
		return nil, nil, err
	}
	var result []map[string]interface{}
	for _, entity := range entities {
		result = append(result, map[string]interface{}{
			"id":         entity.ID,
			"file_id":    entity.FileID,
			"name":       entity.Name,
			"type":       entity.Type,
			"kind":       entity.Kind,
			"signature":  entity.Signature,
			"start_line": entity.StartLine,
			"end_line":   entity.EndLine,
		})
	}
	return textResult(serializeJSON(map[string]interface{}{"entities": result, "count": len(result)})), nil, nil
}

func (s *Server) handleQueryCallGraph(_ context.Context, _ *mcpsdk.CallToolRequest, args queryCallGraphArgs) (*mcpsdk.CallToolResult, any, error) {
	result, err := s.indexer.QueryCallGraph(int64(args.EntityID))
	if err != nil {
		return nil, nil, err
	}
	return textResult(serializeJSON(result)), nil, nil
}

func (s *Server) handleQueryDependencies(_ context.Context, _ *mcpsdk.CallToolRequest, args queryDepsArgs) (*mcpsdk.CallToolResult, any, error) {
	result, err := s.indexer.QueryDependencyGraph(args.Path)
	if err != nil {
		return nil, nil, err
	}
	return textResult(serializeJSON(result)), nil, nil
}

func (s *Server) handleGraphStats(_ context.Context, _ *mcpsdk.CallToolRequest, _ graphStatsArgs) (*mcpsdk.CallToolResult, any, error) {
	stats, err := s.indexer.GetStats()
	if err != nil {
		return nil, nil, err
	}
	return textResult(serializeJSON(stats)), nil, nil
}

func (s *Server) handleGetDocs(_ context.Context, _ *mcpsdk.CallToolRequest, args getDocsArgs) (*mcpsdk.CallToolResult, any, error) {
	scope := args.Scope
	if scope == "" {
		scope = "entity"
	}

	var result interface{}
	var err error
	switch scope {
	case "entity":
		result, err = s.docsForEntity(args.Name, args.FormatHint)
	case "file":
		result, err = s.docsForFile(args.Name, args.FormatHint)
	case "project":
		result, err = s.docsForProject(args.FormatHint)
	default:
		return nil, nil, fmt.Errorf("unknown scope %q: use entity, file, or project", scope)
	}
	if err != nil {
		return nil, nil, err
	}
	return textResult(serializeJSON(result)), nil, nil
}

func (s *Server) handleListFiles(_ context.Context, _ *mcpsdk.CallToolRequest, args listFilesArgs) (*mcpsdk.CallToolResult, any, error) {
	files, err := s.indexer.GetAllFiles()
	if err != nil {
		return nil, nil, err
	}
	var result []map[string]interface{}
	for _, f := range files {
		if args.Language != "" && f.Language != args.Language {
			continue
		}
		result = append(result, map[string]interface{}{
			"path":     f.Path,
			"language": f.Language,
		})
	}
	return textResult(serializeJSON(map[string]interface{}{"files": result, "count": len(result)})), nil, nil
}

func (s *Server) handleGetFileOutline(_ context.Context, _ *mcpsdk.CallToolRequest, args getFileOutlineArgs) (*mcpsdk.CallToolResult, any, error) {
	f, err := s.indexer.GetFileByPath(args.Path)
	if err != nil {
		return nil, nil, err
	}
	if f == nil {
		return nil, nil, fmt.Errorf("file %q not found", args.Path)
	}

	ents, err := s.indexer.GetEntitiesByFile(f.ID)
	if err != nil {
		return nil, nil, err
	}

	var result []map[string]interface{}
	for _, e := range ents {
		qualifiedName := e.Name
		if e.Parent != "" {
			qualifiedName = e.Parent + "." + e.Name
		}
		result = append(result, map[string]interface{}{
			"id":             e.ID,
			"qualified_name": qualifiedName,
			"type":           e.Type,
			"kind":           e.Kind,
			"start_line":     e.StartLine,
			"end_line":       e.EndLine,
		})
	}
	return textResult(serializeJSON(map[string]interface{}{"file": args.Path, "entities": result, "count": len(result)})), nil, nil
}

func (s *Server) handleFindUsages(_ context.Context, _ *mcpsdk.CallToolRequest, args findUsagesArgs) (*mcpsdk.CallToolResult, any, error) {
	// Query entity relations where relation_type="calls" and target_entity_id=args.EntityID
	rels, err := s.indexer.GetAllRelations()
	if err != nil {
		return nil, nil, err
	}

	var callers []map[string]interface{}
	for _, rel := range rels {
		if rel.RelationType == "calls" && rel.TargetEntityID == int64(args.EntityID) {
			if caller, cerr := s.indexer.GetEntityByID(rel.SourceEntityID); cerr == nil && caller != nil {
				callers = append(callers, map[string]interface{}{
					"id":   caller.ID,
					"name": caller.Name,
					"type": caller.Type,
				})
			}
		}
	}
	return textResult(serializeJSON(map[string]interface{}{"entity_id": args.EntityID, "callers": callers, "count": len(callers)})), nil, nil
}

func (s *Server) handleSearchEntities(_ context.Context, _ *mcpsdk.CallToolRequest, args searchEntitiesArgs) (*mcpsdk.CallToolResult, any, error) {
	// Substring search logic requires a new Indexer method, falling back to filtering all entities for now.
	ents, err := s.indexer.GetAllEntities()
	if err != nil {
		return nil, nil, err
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	var results []map[string]interface{}
	for _, e := range ents {
		// Simple case-insensitive match (TODO properly via DB, but this works given current indexer methods without schema changing)
		if len(results) >= limit {
			break
		}
		
		qualifiedName := e.Name
		if e.Parent != "" {
			qualifiedName = e.Parent + "." + e.Name
		}
		
		if strings.Contains(strings.ToLower(qualifiedName), strings.ToLower(args.Query)) {
			results = append(results, map[string]interface{}{
				"id":             e.ID,
				"qualified_name": qualifiedName,
				"type":           e.Type,
				"file_id":        e.FileID,
			})
		}
	}
	return textResult(serializeJSON(map[string]interface{}{"query": args.Query, "results": results, "count": len(results)})), nil, nil
}

func (s *Server) handleGetEntityCode(_ context.Context, _ *mcpsdk.CallToolRequest, args getEntityCodeArgs) (*mcpsdk.CallToolResult, any, error) {
	e, err := s.indexer.GetEntityByID(int64(args.EntityID))
	if err != nil {
		return nil, nil, err
	}
	if e == nil {
		return nil, nil, fmt.Errorf("entity ID %v not found", args.EntityID)
	}
	f, err := s.indexer.GetFileByID(e.FileID)
	if err != nil {
		return nil, nil, err
	}

	contentBytes, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %q: %v", f.Path, err)
	}

	lines := strings.Split(string(contentBytes), "\n")
	
	// Ensure indices are within bounds
	startIdx := e.StartLine - 1
	if startIdx < 0 { startIdx = 0 }
	endIdx := e.EndLine
	if endIdx > len(lines) { endIdx = len(lines) }
	
	if startIdx >= endIdx {
		return textResult(""), nil, nil
	}

	snippet := strings.Join(lines[startIdx:endIdx], "\n")
	return textResult(serializeJSON(map[string]interface{}{
		"entity_id":  e.ID,
		"name":       e.Name,
		"file":       f.Path,
		"start_line": e.StartLine,
		"end_line":   e.EndLine,
		"code":       snippet,
	})), nil, nil
}

func (s *Server) handleGetFileImports(_ context.Context, _ *mcpsdk.CallToolRequest, args getFileImportsArgs) (*mcpsdk.CallToolResult, any, error) {
	f, err := s.indexer.GetFileByPath(args.Path)
	if err != nil {
		return nil, nil, err
	}
	if f == nil {
		return nil, nil, fmt.Errorf("file %q not found", args.Path)
	}

	deps, err := s.indexer.GetAllDependencies()
	if err != nil {
		return nil, nil, err
	}

	var results []string
	for _, dep := range deps {
		if dep.SourceFileID == f.ID {
			results = append(results, dep.TargetPath)
		}
	}

	return textResult(serializeJSON(map[string]interface{}{
		"file":    args.Path,
		"imports": results,
		"count":   len(results),
	})), nil, nil
}

// --------------------------------------------------------------------------
// Docs helpers
// --------------------------------------------------------------------------

func (s *Server) docsForEntity(name, formatHint string) (interface{}, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for scope=entity")
	}
	entities, err := s.indexer.QueryEntity(name)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return map[string]interface{}{
			"scope":       "entity",
			"format_hint": formatHint,
			"results":     []interface{}{},
			"message":     fmt.Sprintf("no entity named %q found", name),
		}, nil
	}

	var results []map[string]interface{}
	for _, e := range entities {
		rec := entityDoc(e)
		if f, ferr := s.indexer.GetFileByID(e.FileID); ferr == nil && f != nil {
			rec["file"] = f.Path
		}
		if rels, rerr := s.indexer.GetEntityRelations(e.ID, "defines"); rerr == nil {
			var members []map[string]interface{}
			for _, r := range rels {
				if child, cerr := s.indexer.GetEntityByID(r.TargetEntityID); cerr == nil && child != nil {
					members = append(members, entityDoc(child))
				}
			}
			rec["members"] = members
		}
		results = append(results, rec)
	}
	return map[string]interface{}{
		"scope": "entity", "format_hint": formatHint, "results": results,
	}, nil
}

func (s *Server) docsForFile(path, formatHint string) (interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("name (file path) is required for scope=file")
	}
	f, err := s.indexer.GetFileByPath(path)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("file %q not found in index", path)
	}
	ents, err := s.indexer.GetEntitiesByFile(f.ID)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	for _, e := range ents {
		rec := entityDoc(e)
		rec["file"] = path
		results = append(results, rec)
	}
	return map[string]interface{}{
		"scope": "file", "format_hint": formatHint, "file": path,
		"entity_count": len(results), "results": results,
	}, nil
}

func (s *Server) docsForProject(formatHint string) (interface{}, error) {
	files, err := s.indexer.GetAllFiles()
	if err != nil {
		return nil, err
	}
	var overview []map[string]interface{}
	for _, f := range files {
		ents, _ := s.indexer.GetEntitiesByFile(f.ID)
		overview = append(overview, map[string]interface{}{
			"file": f.Path, "language": f.Language, "entities": len(ents),
		})
	}
	stats, _ := s.indexer.GetStats()
	return map[string]interface{}{
		"scope": "project", "format_hint": formatHint, "stats": stats, "files": overview,
	}, nil
}

// entityDoc converts a *db.Entity into a flat documentation map.
func entityDoc(e *db.Entity) map[string]interface{} {
	qualifiedName := e.Name
	if e.Parent != "" {
		qualifiedName = e.Parent + "." + e.Name
	}
	return map[string]interface{}{
		"id":             e.ID,
		"name":           e.Name,
		"qualified_name": qualifiedName,
		"type":           e.Type,
		"kind":           e.Kind,
		"signature":      e.Signature,
		"visibility":     e.Visibility,
		"scope":          e.Scope,
		"language":       e.Language,
		"parent":         e.Parent,
		"documentation":  e.Documentation,
		"lines": map[string]interface{}{
			"start": e.StartLine,
			"end":   e.EndLine,
		},
	}
}

// --------------------------------------------------------------------------
// HTTP transport
// --------------------------------------------------------------------------

// ListenHTTP starts an HTTP MCP server using the official SDK's Streamable HTTP handler.
func (s *Server) ListenHTTP(addr string) error {
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(r *http.Request) *mcpsdk.Server {
			return s.inner
		},
		nil,
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	return http.ListenAndServe(addr, mux)
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func textResult(text string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: text},
		},
	}
}

func serializeJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
