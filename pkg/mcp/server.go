package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
)

type Server struct {
	indexer *indexer.Indexer
}

func New(idx *indexer.Indexer) *Server {
	return &Server{indexer: idx}
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// GetTools returns the list of available MCP tools
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
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	Content []map[string]interface{} `json:"content"`
}

// CallTool executes a tool and returns the result
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
		"status": "success",
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
			"id":        entity.ID,
			"file_id":   entity.FileID,
			"name":      entity.Name,
			"type":      entity.Type,
			"kind":      entity.Kind,
			"signature": entity.Signature,
			"start_line": entity.StartLine,
			"end_line":   entity.EndLine,
		})
	}

	return map[string]interface{}{
		"entities": result,
		"count":    len(result),
	}, nil
}

func (s *Server) handleQueryCallGraph(args map[string]interface{}) (interface{}, error) {
	entityID, ok := args["entity_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("entity_id must be a number")
	}

	graph, err := s.indexer.QueryCallGraph(int64(entityID))
	if err != nil {
		return nil, err
	}

	return graph, nil
}

func (s *Server) handleQueryDependencies(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path must be a string")
	}

	deps, err := s.indexer.QueryDependencyGraph(path)
	if err != nil {
		return nil, err
	}

	return deps, nil
}

func (s *Server) handleGraphStats(args map[string]interface{}) (interface{}, error) {
	stats, err := s.indexer.GetStats()
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// SerializeToolResult serializes tool result to JSON
func SerializeToolResult(result interface{}) string {
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
}
