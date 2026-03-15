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
	"path/filepath"
	"sort"
	"strings"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
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

// Routes registers all web UI routes on mux.
// Use this to mount the web UI on a shared mux (e.g. alongside MCP).
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleUI)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/tree", s.handleTree)
	mux.HandleFunc("/api/dir", s.handleDirDetail)
}

// Listen starts the HTTP server on addr (e.g. ":8080").
func (s *Server) Listen(addr string) error {
	mux := http.NewServeMux()
	s.Routes(mux)
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

	nodes := make([]graphNode, 0)
	edges := make([]graphEdge, 0)

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
	// Build a basename → [(path, nodeID)] index so resolution is O(1) per dep
	// instead of O(files) per dep.
	type fileEntry struct {
		path string
		nid  string
	}
	baseIndex := make(map[string][]fileEntry)
	for _, f := range files {
		base := filepath.Base(f.Path)
		baseIndex[base] = append(baseIndex[base], fileEntry{f.Path, fileIDToNodeID[f.ID]})
	}

	for _, dep := range deps {
		srcNode, ok := fileIDToNodeID[dep.SourceFileID]
		if !ok {
			continue
		}
		// Resolve by trying the import path's basename (with common extensions).
		importBase := filepath.Base(dep.TargetPath)
		// Strip leading dots from relative imports.
		for len(importBase) > 0 && importBase[0] == '.' {
			importBase = importBase[1:]
		}
		if importBase == "" {
			continue
		}
		candidates := []string{importBase}
		if filepath.Ext(importBase) == "" {
			for _, ext := range []string{".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java"} {
				candidates = append(candidates, importBase+ext)
			}
		}
		var tgtNode string
		for _, cand := range candidates {
			for _, entry := range baseIndex[cand] {
				if pathSuffixMatch(entry.path, dep.TargetPath) {
					tgtNode = entry.nid
					break
				}
			}
			if tgtNode != "" {
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

// --------------------------------------------------------------------------
// Tree endpoint — lightweight directory hierarchy for the icicle chart
// --------------------------------------------------------------------------

// treeNode is a node in the directory/file hierarchy returned by /api/tree.
type treeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Count    int         `json:"count"` // total files under this node
	Lang     string      `json:"lang,omitempty"`
	Children []*treeNode `json:"children,omitempty"`
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	files, err := s.idx.GetAllFiles()
	if err != nil {
		http.Error(w, `{"error":"failed to load files"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(buildFileTree(files))
}

func buildFileTree(files []*db.File) *treeNode {
	type trieN struct {
		path     string
		lang     string // set only for leaf (file) nodes
		domLang  string // dominant language in subtree
		count    int
		children map[string]*trieN
	}

	root := &trieN{path: "", children: map[string]*trieN{}}

	for _, f := range files {
		parts := splitPath(f.Path)
		cur := root
		cur.count++
		for i, part := range parts {
			child, ok := cur.children[part]
			if !ok {
				var p string
				if cur.path == "" {
					p = part
				} else {
					p = cur.path + "/" + part
				}
				child = &trieN{path: p, children: map[string]*trieN{}}
				cur.children[part] = child
			}
			child.count++
			if i == len(parts)-1 {
				child.lang = f.Language
			}
			cur = child
		}
	}

	// Compute dominant language bottom-up.
	var dominantLang func(*trieN) string
	dominantLang = func(n *trieN) string {
		if n.lang != "" {
			n.domLang = n.lang
			return n.lang
		}
		counts := map[string]int{}
		for _, child := range n.children {
			l := dominantLang(child)
			if l != "" {
				counts[l] += child.count
			}
		}
		best, bestN := "", 0
		for l, c := range counts {
			if c > bestN {
				bestN = c
				best = l
			}
		}
		n.domLang = best
		return best
	}
	dominantLang(root)

	var convert func(*trieN, string) *treeNode
	convert = func(n *trieN, name string) *treeNode {
		tn := &treeNode{Name: name, Path: n.path, Count: n.count, Lang: n.domLang}
		for childName, child := range n.children {
			tn.Children = append(tn.Children, convert(child, childName))
		}
		sort.Slice(tn.Children, func(i, j int) bool {
			return tn.Children[i].Count > tn.Children[j].Count
		})
		return tn
	}
	return convert(root, ".")
}

// --------------------------------------------------------------------------
// Dir detail endpoint — dependency & entity summary for a directory/file
// --------------------------------------------------------------------------

type dirDetail struct {
	Path        string        `json:"path"`
	FileCount   int           `json:"fileCount"`
	ImportsFrom []string      `json:"importsFrom"`
	ImportedBy  []string      `json:"importedBy"`
	TopFiles    []string      `json:"topFiles"`
	TopEntities []entityBrief `json:"topEntities"`
}

type entityBrief struct {
	Name string `json:"name"`
	Type string `json:"type"`
	File string `json:"file"`
}

func (s *Server) handleDirDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	dirPath := r.URL.Query().Get("path")

	files, err := s.idx.GetAllFiles()
	if err != nil {
		http.Error(w, `{"error":"failed to load files"}`, http.StatusInternalServerError)
		return
	}

	fileByID := make(map[int64]*db.File, len(files))
	var dirFiles []*db.File
	dirFileIDs := map[int64]bool{}
	for _, f := range files {
		fileByID[f.ID] = f
		if isUnderPath(f.Path, dirPath) {
			dirFiles = append(dirFiles, f)
			dirFileIDs[f.ID] = true
		}
	}

	deps, err := s.idx.GetAllDependencies()
	if err != nil {
		http.Error(w, `{"error":"failed to load deps"}`, http.StatusInternalServerError)
		return
	}

	// Build basename → [path…] and path → id indexes for import resolution.
	baseToPaths := map[string][]string{}
	pathToID := map[string]int64{}
	for _, f := range files {
		base := filepath.Base(f.Path)
		baseToPaths[base] = append(baseToPaths[base], f.Path)
		pathToID[f.Path] = f.ID
	}

	importFrom := map[string]bool{}
	importedBy := map[string]bool{}

	for _, dep := range deps {
		targetID := resolveDepToID(dep.TargetPath, baseToPaths, pathToID)
		if dirFileIDs[dep.SourceFileID] {
			// Outgoing: this dir's file imports something outside.
			if targetID != 0 {
				if tf, ok := fileByID[targetID]; ok && !dirFileIDs[tf.ID] {
					importFrom[dirOfPath(tf.Path)] = true
				}
			}
		} else if targetID != 0 && dirFileIDs[targetID] {
			// Incoming: another file imports something in this dir.
			if sf, ok := fileByID[dep.SourceFileID]; ok {
				importedBy[dirOfPath(sf.Path)] = true
			}
		}
	}

	// Top files (first 20).
	topFiles := make([]string, 0, 20)
	for i, f := range dirFiles {
		if i >= 20 {
			break
		}
		topFiles = append(topFiles, filepath.Base(f.Path))
	}

	// Top entities (scan first 10 files, cap at 30 entities).
	var topEntities []entityBrief
	for i, f := range dirFiles {
		if i >= 10 || len(topEntities) >= 30 {
			break
		}
		ents, err := s.idx.GetEntitiesByFile(f.ID)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if len(topEntities) >= 30 {
				break
			}
			topEntities = append(topEntities, entityBrief{Name: e.Name, Type: e.Type, File: filepath.Base(f.Path)})
		}
	}

	_ = json.NewEncoder(w).Encode(dirDetail{
		Path:        dirPath,
		FileCount:   len(dirFiles),
		ImportsFrom: sortedKeys(importFrom),
		ImportedBy:  sortedKeys(importedBy),
		TopFiles:    topFiles,
		TopEntities: topEntities,
	})
}

func isUnderPath(filePath, dirPath string) bool {
	if dirPath == "" || dirPath == "." {
		return true
	}
	return filePath == dirPath ||
		strings.HasPrefix(filePath, dirPath+"/") ||
		strings.HasPrefix(filePath, dirPath+"\\")
}

func dirOfPath(p string) string {
	parts := splitPath(p)
	if len(parts) <= 1 {
		return "."
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func resolveDepToID(targetPath string, baseToPaths map[string][]string, pathToID map[string]int64) int64 {
	norm := strings.ReplaceAll(targetPath, ".", "/")
	base := filepath.Base(norm)
	if base == "" || base == "." {
		return 0
	}
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		for _, ext := range []string{".java", ".go", ".py", ".js", ".ts"} {
			candidates = append(candidates, base+ext)
		}
	}
	for _, cand := range candidates {
		for _, p := range baseToPaths[cand] {
			if pathSuffixMatch(p, norm) {
				return pathToID[p]
			}
		}
	}
	return 0
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
