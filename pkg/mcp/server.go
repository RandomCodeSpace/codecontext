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

func (s *Server) registerTools() {
	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "index_directory",
		Description: "Index a directory to build the code graph",
	}, s.handleIndexDirectory)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_entity",
		Description: "Query for an entity by name",
	}, s.handleQueryEntity)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_call_graph",
		Description: "Get the call graph for a specific entity",
	}, s.handleQueryCallGraph)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "query_dependencies",
		Description: "Get dependencies for a file",
	}, s.handleQueryDependencies)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "graph_stats",
		Description: "Get statistics about the indexed code graph",
	}, s.handleGraphStats)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name: "get_docs",
		Description: "Return structured documentation for a named entity, a file, or the " +
			"whole project. The result is raw structured data; pass format_hint to tell the " +
			"calling agent how to reformat it (e.g. 'Markdown', 'JSDoc', 'OpenAPI', 'plain').",
	}, s.handleGetDocs)
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
