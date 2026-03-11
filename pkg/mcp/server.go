package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
)

// Server is the MCP server.  It is transport-agnostic; callers drive it via
// Dispatch and use ListenHTTP or their own stdio loop.
type Server struct {
	indexer *indexer.Indexer
	version string
	logger  *slog.Logger
}

func New(idx *indexer.Indexer, version string) *Server {
	return &Server{
		indexer: idx,
		version: version,
		logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

// SetLogger replaces the default JSON-to-stderr logger.
func (s *Server) SetLogger(l *slog.Logger) { s.logger = l }

// --------------------------------------------------------------------------
// Tool catalogue
// --------------------------------------------------------------------------

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// GetTools returns the list of available MCP tools.
func (s *Server) GetTools() []Tool {
	return []Tool{
		{
			Name:        "index_directory",
			Description: "Index a directory to build the code graph",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the directory to index",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "query_entity",
			Description: "Query for an entity by name",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the entity to search for",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "query_call_graph",
			Description: "Get the call graph for a specific entity",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"entity_id": map[string]interface{}{
						"type":        "number",
						"description": "ID of the entity to get call graph for",
					},
				},
				"required": []string{"entity_id"},
			},
		},
		{
			Name:        "query_dependencies",
			Description: "Get dependencies for a file",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path of the file to query",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "graph_stats",
			Description: "Get statistics about the indexed code graph",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name: "get_docs",
			Description: "Return structured documentation for a named entity, a file, or the " +
				"whole project. The result is raw structured data; pass format_hint to tell the " +
				"calling agent how to reformat it (e.g. 'Markdown', 'JSDoc', 'OpenAPI', 'plain').",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Entity name (scope=entity) or file path (scope=file). Omit for scope=project.",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"entity", "file", "project"},
						"description": "entity: single named symbol; file: all symbols in one file; project: whole-codebase overview. Defaults to 'entity'.",
					},
					"format_hint": map[string]interface{}{
						"type":        "string",
						"description": "Optional hint for the agent reformatting this output, e.g. 'Markdown', 'JSDoc', 'OpenAPI', 'plain'.",
					},
				},
				"required": []string{},
			},
		},
	}
}

// --------------------------------------------------------------------------
// Dispatch — unified JSON-RPC method handler with logging
// --------------------------------------------------------------------------

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Content []map[string]interface{} `json:"content"`
}

// Dispatch handles a single JSON-RPC method, logs it, and returns the result.
// It is used by both the stdio loop in main.go and the HTTP handler below.
func (s *Server) Dispatch(method string, params map[string]interface{}) (interface{}, error) {
	start := time.Now()

	result, err := s.dispatch(method, params)

	attrs := []any{
		"method", method,
		"duration_ms", time.Since(start).Milliseconds(),
	}
	if method == "tools/call" {
		attrs = append(attrs, "tool", params["name"])
	}
	if err != nil {
		s.logger.Error("mcp", append(attrs, "error", err.Error())...)
	} else {
		s.logger.Info("mcp", attrs...)
	}

	return result, err
}

func (s *Server) dispatch(method string, params map[string]interface{}) (interface{}, error) {
	switch method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "codecontext", "version": s.version},
		}, nil

	case "tools/list":
		return map[string]interface{}{"tools": s.GetTools()}, nil

	case "tools/call":
		toolName, _ := params["name"].(string)
		arguments, _ := params["arguments"].(map[string]interface{})
		if arguments == nil {
			arguments = map[string]interface{}{}
		}
		result, err := s.CallTool(toolName, arguments)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": SerializeToolResult(result)},
			},
		}, nil

	default:
		return nil, fmt.Errorf("method not found: %s", method)
	}
}

// --------------------------------------------------------------------------
// Tool handlers
// --------------------------------------------------------------------------

// CallTool executes a tool and returns the result.
func (s *Server) CallTool(toolName string, args map[string]interface{}) (interface{}, error) {
	switch toolName {
	case "index_directory":
		return s.handleIndexDirectory(args)
	case "query_entity":
		return s.handleQueryEntity(args)
	case "query_call_graph":
		return s.handleQueryCallGraph(args)
	case "query_dependencies":
		return s.handleQueryDependencies(args)
	case "graph_stats":
		return s.handleGraphStats(args)
	case "get_docs":
		return s.handleGetDocs(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (s *Server) handleIndexDirectory(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path must be a string")
	}
	if err := s.indexer.IndexDirectory(path); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Successfully indexed directory: %s", path),
	}, nil
}

func (s *Server) handleQueryEntity(args map[string]interface{}) (interface{}, error) {
	name, ok := args["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name must be a string")
	}
	entities, err := s.indexer.QueryEntity(name)
	if err != nil {
		return nil, err
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
	return map[string]interface{}{"entities": result, "count": len(result)}, nil
}

func (s *Server) handleQueryCallGraph(args map[string]interface{}) (interface{}, error) {
	entityID, ok := args["entity_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("entity_id must be a number")
	}
	return s.indexer.QueryCallGraph(int64(entityID))
}

func (s *Server) handleQueryDependencies(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path must be a string")
	}
	return s.indexer.QueryDependencyGraph(path)
}

func (s *Server) handleGraphStats(args map[string]interface{}) (interface{}, error) {
	stats, err := s.indexer.GetStats()
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// handleGetDocs returns rich structured documentation that an agentic caller
// can reformat according to format_hint.
//
//   scope="entity"  (default) — all entities matching name, with file path,
//                               signature, docs, visibility, and child members
//   scope="file"    — all entities in the file at the given path
//   scope="project" — overview: list of files with entity/dependency counts
func (s *Server) handleGetDocs(args map[string]interface{}) (interface{}, error) {
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "entity"
	}
	name, _ := args["name"].(string)
	formatHint, _ := args["format_hint"].(string)

	switch scope {
	case "entity":
		return s.docsForEntity(name, formatHint)
	case "file":
		return s.docsForFile(name, formatHint)
	case "project":
		return s.docsForProject(formatHint)
	default:
		return nil, fmt.Errorf("unknown scope %q: use entity, file, or project", scope)
	}
}

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

		// Attach file path.
		if f, ferr := s.indexer.GetFileByID(e.FileID); ferr == nil && f != nil {
			rec["file"] = f.Path
		}

		// Attach child members (entities this one "defines").
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
		"scope":       "entity",
		"format_hint": formatHint,
		"results":     results,
	}, nil
}

func (s *Server) docsForFile(path, formatHint string) (interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("name (file path) is required for scope=file")
	}

	// Get file by path via indexer files list.
	files, err := s.indexer.GetAllFiles()
	if err != nil {
		return nil, err
	}
	var fileID int64
	for _, f := range files {
		if f.Path == path {
			fileID = f.ID
			break
		}
	}
	if fileID == 0 {
		return nil, fmt.Errorf("file %q not found in index", path)
	}

	ents, err := s.indexer.GetEntitiesByFile(fileID)
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
		"scope":        "file",
		"format_hint":  formatHint,
		"file":         path,
		"entity_count": len(results),
		"results":      results,
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
			"file":     f.Path,
			"language": f.Language,
			"entities": len(ents),
		})
	}

	stats, _ := s.indexer.GetStats()
	return map[string]interface{}{
		"scope":       "project",
		"format_hint": formatHint,
		"stats":       stats,
		"files":       overview,
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

// ListenHTTP starts an HTTP MCP server on addr (e.g. ":8081").
//
// Endpoints:
//
//	POST /mcp          JSON-RPC 2.0 request → response
//	GET  /mcp/tools    convenience: list tools as JSON (no JSON-RPC wrapper)
func (s *Server) ListenHTTP(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.httpMCP)
	mux.HandleFunc("/mcp/tools", s.httpTools)
	return http.ListenAndServe(addr, mux)
}

// httpMCP handles POST /mcp — standard JSON-RPC 2.0 over HTTP.
func (s *Server) httpMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeHTTPError(w, nil, -32700, "parse error")
		return
	}

	id := req["id"]
	method, _ := req["method"].(string)
	params, _ := req["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")

	result, err := s.Dispatch(method, params)
	if err != nil {
		s.writeHTTPError(w, id, -32603, err.Error())
		return
	}
	s.writeHTTPResult(w, id, result)
}

// httpTools handles GET /mcp/tools — plain JSON tool listing.
func (s *Server) httpTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"tools": s.GetTools()})
}

func (s *Server) writeHTTPResult(w http.ResponseWriter, id, result interface{}) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0", "id": id, "result": result,
	})
}

func (s *Server) writeHTTPError(w http.ResponseWriter, id interface{}, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]interface{}{"code": code, "message": message},
	})
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// SerializeToolResult serializes a tool result to indented JSON.
func SerializeToolResult(result interface{}) string {
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
}
