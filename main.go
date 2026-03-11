package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RandomCodeSpace/codecontext/pkg/ai"
	"github.com/RandomCodeSpace/codecontext/pkg/db"
	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
	"github.com/RandomCodeSpace/codecontext/pkg/llm"
	"github.com/RandomCodeSpace/codecontext/pkg/mcp"
)

var version = "dev"

const usage = `codecontext - aggregate source files and build code graphs with AI analysis

Usage:
  codecontext [flags] [command] [args...]

Commands:
  (no command)         Aggregate source files into a single context block
  index                Index a directory to build code graph
  query                Query the code graph
  ai                   Analyze code with AI
  stats                Show graph statistics
  mcp                  Start MCP server

Flags:
  -ext string          Comma-separated file extensions to include (e.g. .go,.ts)
  -graph string        Path to graph database (default: .codecontext.db)
  -version             Print version and exit
  -help                Print this help message

AI Subcommands:
  codecontext ai query <query>           Ask a natural language question about the code
  codecontext ai analyze <entity>        Detailed analysis of an entity
  codecontext ai docs <entity>           Generate documentation for an entity
  codecontext ai review <entity>         Code review suggestions for an entity
  codecontext ai summarize <file>        Summary of a file's purpose
  codecontext ai chat                    Multi-turn conversation about code

Examples:
  codecontext .                              # aggregate all files in current directory
  codecontext -ext .go,.md .                 # aggregate only .go and .md files
  codecontext index .                        # index current directory
  codecontext query entity myFunction        # query for entity
  codecontext stats                          # show graph statistics
  codecontext ai query "what does main do"  # AI analysis
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
	case "ai":
		handleAI(graphDB, cmdArgs)
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

	// MCP JSON-RPC 2.0 server over stdin/stdout
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req map[string]interface{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeJSONRPCError(nil, -32700, "Parse error")
			continue
		}

		id := req["id"]
		method, _ := req["method"].(string)

		switch method {
		case "initialize":
			writeJSONRPCResult(id, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "codecontext", "version": version},
			})

		case "tools/list":
			tools := mcpServer.GetTools()
			writeJSONRPCResult(id, map[string]interface{}{"tools": tools})

		case "tools/call":
			params, _ := req["params"].(map[string]interface{})
			toolName, _ := params["name"].(string)
			arguments, _ := params["arguments"].(map[string]interface{})
			if arguments == nil {
				arguments = map[string]interface{}{}
			}
			result, err := mcpServer.CallTool(toolName, arguments)
			if err != nil {
				writeJSONRPCError(id, -32603, err.Error())
				continue
			}
			writeJSONRPCResult(id, map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": mcp.SerializeToolResult(result)},
				},
			})

		default:
			writeJSONRPCError(id, -32601, "Method not found")
		}
	}
}

func writeJSONRPCResult(id interface{}, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func writeJSONRPCError(id interface{}, code int, message string) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
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

func handleAI(graphDB *string, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: codecontext ai <subcommand> [args]\n")
		fmt.Fprintf(os.Stderr, "subcommands: query, analyze, docs, review, summarize, chat\n")
		os.Exit(1)
	}

	subcommand := args[0]
	subargs := args[1:]

	// Open database
	database, err := db.Open(*graphDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Load LLM configuration
	cfg := llm.LoadConfig()

	// Create LLM provider
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating LLM provider: %v\n", err)
		os.Exit(1)
	}

	// Check if provider is healthy
	ctx := context.Background()
	healthy, err := provider.IsHealthy(ctx)
	if err != nil || !healthy {
		fmt.Fprintf(os.Stderr, "error: LLM provider is not available\n")
		fmt.Fprintf(os.Stderr, "make sure your LLM provider is running and properly configured\n")
		os.Exit(1)
	}

	// Create indexer and AI chain
	idx := indexer.New(database)
	chain := ai.NewChain(idx, provider)

	switch subcommand {
	case "query":
		if len(subargs) == 0 {
			fmt.Fprintf(os.Stderr, "usage: codecontext ai query <question>\n")
			os.Exit(1)
		}
		question := strings.Join(subargs, " ")
		response, err := chain.QueryNatural(ctx, question)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "analyze":
		if len(subargs) == 0 {
			fmt.Fprintf(os.Stderr, "usage: codecontext ai analyze <entity_name>\n")
			os.Exit(1)
		}
		entityName := subargs[0]
		response, err := chain.AnalyzeEntity(ctx, entityName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "docs":
		if len(subargs) == 0 {
			fmt.Fprintf(os.Stderr, "usage: codecontext ai docs <entity_name>\n")
			os.Exit(1)
		}
		entityName := subargs[0]
		response, err := chain.GenerateDocs(ctx, entityName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "review":
		if len(subargs) == 0 {
			fmt.Fprintf(os.Stderr, "usage: codecontext ai review <entity_name>\n")
			os.Exit(1)
		}
		entityName := subargs[0]
		response, err := chain.ReviewCode(ctx, entityName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "summarize":
		if len(subargs) == 0 {
			fmt.Fprintf(os.Stderr, "usage: codecontext ai summarize <file_path>\n")
			os.Exit(1)
		}
		filePath := subargs[0]
		response, err := chain.Summarize(ctx, filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "chat":
		handleAIChat(ctx, chain)

	default:
		fmt.Fprintf(os.Stderr, "unknown AI subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func handleAIChat(ctx context.Context, chain *ai.Chain) {
	fmt.Println("AI Chat - Interactive conversation about code")
	fmt.Println("Commands:")
	fmt.Println("  analyze <entity>  - Analyze a specific entity")
	fmt.Println("  docs <entity>     - Generate docs for entity")
	fmt.Println("  review <entity>   - Review code")
	fmt.Println("  exit              - Exit chat")
	fmt.Println()

	conversation := &ai.ConversationContext{
		Messages: []*ai.Message{
			{
				Role: "system",
				Content: "You are a helpful code analysis assistant. You have access to a codebase and can help analyze, explain, and improve code.",
			},
		},
	}

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		input := scanner.Text()

		if input == "exit" {
			fmt.Println("Goodbye!")
			return
		}

		// Handle special commands
		parts := strings.Fields(input)
		if len(parts) > 0 {
			switch parts[0] {
			case "analyze":
				if len(parts) < 2 {
					fmt.Println("usage: analyze <entity_name>")
					fmt.Print("> ")
					continue
				}
				response, err := chain.AnalyzeEntity(ctx, parts[1])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					fmt.Println(response)
				}
				fmt.Print("> ")
				continue

			case "docs":
				if len(parts) < 2 {
					fmt.Println("usage: docs <entity_name>")
					fmt.Print("> ")
					continue
				}
				response, err := chain.GenerateDocs(ctx, parts[1])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					fmt.Println(response)
				}
				fmt.Print("> ")
				continue

			case "review":
				if len(parts) < 2 {
					fmt.Println("usage: review <entity_name>")
					fmt.Print("> ")
					continue
				}
				response, err := chain.ReviewCode(ctx, parts[1])
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					fmt.Println(response)
				}
				fmt.Print("> ")
				continue
			}
		}

		// Regular chat message
		response, err := chain.Chat(ctx, conversation, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Println(response)
		}

		fmt.Print("> ")
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
	}
}
