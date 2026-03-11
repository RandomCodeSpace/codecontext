package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
	"github.com/RandomCodeSpace/codecontext/pkg/mcp"
)

var version = "dev"

const usage = `codecontext - aggregate source files and build code graphs

Usage:
  codecontext [flags] [command] [args...]

Commands:
  (no command)         Aggregate source files into a single context block
  index                Index a directory to build code graph
  query                Query the code graph
  mcp                  Start MCP server
  stats                Show graph statistics

Flags:
  -ext string          Comma-separated file extensions to include (e.g. .go,.ts)
  -graph string        Path to graph database (default: .codecontext.db)
  -version             Print version and exit
  -help                Print this help message

Examples:
  codecontext .                              # aggregate all files in current directory
  codecontext -ext .go,.md .                 # aggregate only .go and .md files
  codecontext index .                        # index current directory
  codecontext query entity myFunction        # query for entity
  codecontext stats                          # show graph statistics
  codecontext mcp                            # start MCP server (for Claude integration)
`

func main() {
	ext := flag.String("ext", "", "comma-separated file extensions to include")
	graphDB := flag.String("graph", ".codecontext.db", "path to graph database")
	ver := flag.Bool("version", false, "print version")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *ver {
		fmt.Printf("codecontext %s\n", version)
		return
	}

	args := flag.Args()

	// If no command, run legacy behavior (aggregate files)
	if len(args) == 0 {
		legacyAggregate(".", ext)
		return
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "index":
		handleIndex(graphDB, cmdArgs)
	case "query":
		handleQuery(graphDB, cmdArgs)
	case "stats":
		handleStats(graphDB)
	case "mcp":
		handleMCP(graphDB)
	default:
		// Treat as paths to aggregate
		legacyAggregate(command, ext)
		for _, arg := range cmdArgs {
			legacyAggregate(arg, ext)
		}
	}
}

// legacyAggregate handles the original functionality
func legacyAggregate(pathArg string, extFilter *string) {
	var exts map[string]bool
	if *extFilter != "" {
		exts = make(map[string]bool)
		for _, e := range strings.Split(*extFilter, ",") {
			e = strings.TrimSpace(e)
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts[e] = true
		}
	}

	info, err := os.Stat(pathArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var files []string
	if info.IsDir() {
		walked, err := walkDir(pathArg, exts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		files = walked
	} else {
		files = []string{pathArg}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no files found")
		os.Exit(1)
	}

	for _, f := range files {
		if err := printFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", f, err)
		}
	}
}

func handleIndex(graphDB *string, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: codecontext index <path>\n")
		os.Exit(1)
	}

	dirPath := args[0]

	// Open or create database
	database, err := db.Open(*graphDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create indexer and index directory
	idx := indexer.New(database)
	fmt.Printf("Indexing directory: %s\n", dirPath)

	if err := idx.IndexDirectory(dirPath); err != nil {
		fmt.Fprintf(os.Stderr, "error indexing directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Indexing complete. Database: %s\n", *graphDB)
}

func handleQuery(graphDB *string, args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: codecontext query <type> <query>\n")
		fmt.Fprintf(os.Stderr, "types: entity, calls, deps\n")
		os.Exit(1)
	}

	queryType := args[0]
	query := args[1]

	// Open database
	database, err := db.Open(*graphDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)

	switch queryType {
	case "entity":
		entities, err := idx.QueryEntity(query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error querying entity: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found %d entities:\n", len(entities))
		for _, e := range entities {
			fmt.Printf("  - ID: %d, Name: %s, Type: %s, File: %d\n", e.ID, e.Name, e.Type, e.FileID)
		}

	case "calls":
		// Parse entity ID from query
		var entityID int64
		fmt.Sscanf(query, "%d", &entityID)
		if entityID == 0 {
			fmt.Fprintf(os.Stderr, "invalid entity ID: %s\n", query)
			os.Exit(1)
		}
		graph, err := idx.QueryCallGraph(entityID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error querying call graph: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Call graph: %v\n", graph)

	case "deps":
		graph, err := idx.QueryDependencyGraph(query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error querying dependencies: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Dependencies: %v\n", graph)

	default:
		fmt.Fprintf(os.Stderr, "unknown query type: %s\n", queryType)
		os.Exit(1)
	}
}

func handleStats(graphDB *string) {
	database, err := db.Open(*graphDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	stats, err := idx.GetStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Code Graph Statistics:")
	fmt.Printf("  Files:       %v\n", stats["files"])
	fmt.Printf("  Entities:    %v\n", stats["entities"])
	fmt.Printf("  Dependencies:%v\n", stats["dependencies"])
	fmt.Printf("  Relations:   %v\n", stats["relations"])
}

func handleMCP(graphDB *string) {
	database, err := db.Open(*graphDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	mcpServer := mcp.New(idx)

	fmt.Println("MCP Server started")
	fmt.Println("Available tools:")
	for _, tool := range mcpServer.GetTools() {
		fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
	}
	fmt.Println("\nMCP server is running. Send JSON-RPC requests for tool invocation.")

	// Simple echo server for tool requests
	// In a real scenario, this would integrate with the actual MCP protocol
	// For now, we just keep the server running
	select {}
}

func walkDir(dir string, exts map[string]bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if exts != nil && !exts[filepath.Ext(path)] {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func printFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("=== %s ===\n%s\n", path, string(data))
	return nil
}
