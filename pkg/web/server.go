// Package web serves the interactive code-graph visualisation UI.
//
// The server exposes:
//
//	GET /         → embedded single-page HTML/JS application
//	GET /api/graph → JSON: {nodes, edges} for the force-directed graph
//	GET /api/stats → JSON: {files, entities, dependencies, relations}
package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
)

// Server wraps an Indexer and serves the web UI.
type Server struct {
	idx *indexer.Indexer
}

// New creates a new web Server backed by the given Indexer.
func New(idx *indexer.Indexer) *Server {
	return &Server{idx: idx}
}

// Listen starts the HTTP server on addr (e.g. ":8080").
func (s *Server) Listen(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/stats", s.handleStats)
	return http.ListenAndServe(addr, mux)
}

// --------------------------------------------------------------------------
// API handlers
// --------------------------------------------------------------------------

type graphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"`  // "file" | "function" | "method" | "class" | …
	Group    string `json:"group"` // "file" | "entity"
	FilePath string `json:"filePath,omitempty"`
	Parent   string `json:"parent,omitempty"`
	Line     int    `json:"line,omitempty"`
}

type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "defines" | "imports"
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	files, err := s.idx.GetAllFiles()
	if err != nil {
		http.Error(w, `{"error":"failed to load files"}`, http.StatusInternalServerError)
		return
	}
	entities, err := s.idx.GetAllEntities()
	if err != nil {
		http.Error(w, `{"error":"failed to load entities"}`, http.StatusInternalServerError)
		return
	}
	relations, err := s.idx.GetAllRelations()
	if err != nil {
		http.Error(w, `{"error":"failed to load relations"}`, http.StatusInternalServerError)
		return
	}
	deps, err := s.idx.GetAllDependencies()
	if err != nil {
		http.Error(w, `{"error":"failed to load dependencies"}`, http.StatusInternalServerError)
		return
	}

	var nodes []graphNode
	var edges []graphEdge

	// File nodes.
	fileIDToNodeID := make(map[int64]string)
	for _, f := range files {
		nid := fmt.Sprintf("f-%d", f.ID)
		fileIDToNodeID[f.ID] = nid
		nodes = append(nodes, graphNode{
			ID:       nid,
			Label:    shortPath(f.Path),
			Type:     f.Language,
			Group:    "file",
			FilePath: f.Path,
		})
	}

	// Entity nodes.
	entityIDToNodeID := make(map[int64]string)
	for _, e := range entities {
		nid := fmt.Sprintf("e-%d", e.ID)
		entityIDToNodeID[e.ID] = nid
		nodes = append(nodes, graphNode{
			ID:       nid,
			Label:    e.Name,
			Type:     e.Type,
			Group:    "entity",
			FilePath: fileIDToNodeID[e.FileID],
			Parent:   e.Parent,
			Line:     e.StartLine,
		})
		// Edge: file → entity (contains).
		if fnid, ok := fileIDToNodeID[e.FileID]; ok {
			edges = append(edges, graphEdge{
				Source: fnid,
				Target: nid,
				Type:   "contains",
			})
		}
	}

	// Relation edges (e.g. "defines").
	for _, rel := range relations {
		src, srcOK := entityIDToNodeID[rel.SourceEntityID]
		tgt, tgtOK := entityIDToNodeID[rel.TargetEntityID]
		if srcOK && tgtOK {
			edges = append(edges, graphEdge{
				Source: src,
				Target: tgt,
				Type:   rel.RelationType,
			})
		}
	}

	// Dependency edges between files (best-effort: resolve by target path suffix).
	pathToFileNode := make(map[string]string)
	for _, f := range files {
		pathToFileNode[f.Path] = fileIDToNodeID[f.ID]
	}
	for _, dep := range deps {
		srcNode, ok := fileIDToNodeID[dep.SourceFileID]
		if !ok {
			continue
		}
		// Try to find a matching file node by path suffix.
		tgtNode := ""
		for path, nid := range pathToFileNode {
			if pathSuffixMatch(path, dep.TargetPath) {
				tgtNode = nid
				break
			}
		}
		if tgtNode != "" {
			edges = append(edges, graphEdge{
				Source: srcNode,
				Target: tgtNode,
				Type:   "imports",
			})
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	stats, err := s.idx.GetStats()
	if err != nil {
		http.Error(w, `{"error":"failed to load stats"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(stats)
}

// handleUI serves the embedded single-page application.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, uiHTML)
}

// --------------------------------------------------------------------------
// Utilities
// --------------------------------------------------------------------------

func shortPath(p string) string {
	// Return the last two path segments for readability.
	parts := splitPath(p)
	if len(parts) <= 2 {
		return p
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func splitPath(p string) []string {
	var parts []string
	cur := ""
	for _, c := range p {
		if c == '/' || c == '\\' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

// pathSuffixMatch returns true if the file path ends with the import path.
func pathSuffixMatch(filePath, importPath string) bool {
	if importPath == "" {
		return false
	}
	// Strip leading ./ or ../
	for len(importPath) > 0 && (importPath[0] == '.' || importPath[0] == '/') {
		importPath = importPath[1:]
	}
	if importPath == "" {
		return false
	}
	// Check if filePath ends with importPath (with or without extension).
	for _, suffix := range []string{importPath, importPath + ".go", importPath + ".py", importPath + ".js", importPath + ".ts"} {
		if len(filePath) >= len(suffix) && filePath[len(filePath)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
