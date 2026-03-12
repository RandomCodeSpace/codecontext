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
	ccmcp "github.com/RandomCodeSpace/codecontext/pkg/mcp"
	"github.com/RandomCodeSpace/codecontext/pkg/web"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
  docs                 Generate documentation for the whole project
  stats                Show graph statistics
  web                  Start web UI to visualise the code graph
  mcp                  Start MCP server (for Claude integration)

Flags:
  -ext string          Comma-separated file extensions to include (e.g. .go,.ts)
  -graph string        Path to graph database (default: .codecontext.db)
  -verbose             Enable verbose logging
  -version             Print version and exit
  -help                Print this help message

Docs Flags (codecontext docs):
  -ai                  Use AI to write entity descriptions
  -prompt string       Custom instruction for AI output style (only with -ai)
                       e.g. "Use JSDoc format" or "One sentence per entity"
  -output string       Write documentation to this file (default: stdout)

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
  codecontext index -verbose .              # index with detailed logging
  codecontext query entity myFunction        # query for entity
  codecontext docs                           # standard Markdown docs to stdout
  codecontext docs -output docs.md           # write standard docs to file
  codecontext docs -ai                       # AI-generated docs (default style)
  codecontext docs -ai -prompt "Use JSDoc"   # AI docs with custom style
  codecontext stats                          # show graph statistics
  codecontext web                            # open graph visualisation at http://localhost:8080
  codecontext ai query "what does main do"  # AI analysis
  codecontext mcp                            # start MCP server (stdio)
  codecontext mcp -http                      # start MCP server (HTTP on :8081)
  codecontext mcp -http -addr :9000          # start MCP server (HTTP on :9000)
`

func main() {
	ext := flag.String("ext", "", "comma-separated file extensions to include")
	graphDB := flag.String("graph", ".codecontext.db", "path to graph database")
	ver := flag.Bool("version", false, "print version")
	verbose := flag.Bool("verbose", false, "enable verbose logging")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *ver {
		fmt.Printf("codecontext %s\n", version)
		return
	}

	args := flag.Args()

	if len(args) == 0 {
		legacyAggregate(".", ext)
		return
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "index":
		handleIndex(graphDB, cmdArgs, *verbose)
	case "query":
		handleQuery(graphDB, cmdArgs, *verbose)
	case "ai":
		handleAI(graphDB, cmdArgs, *verbose)
	case "docs":
		handleDocs(graphDB, cmdArgs, *verbose)
	case "stats":
		handleStats(graphDB, *verbose)
	case "web":
		handleWeb(graphDB, cmdArgs, *verbose)
	case "mcp":
		handleMCP(graphDB, cmdArgs)
	default:
		legacyAggregate(command, ext)
		for _, arg := range cmdArgs {
			legacyAggregate(arg, ext)
		}
	}
}

// --------------------------------------------------------------------------
// Legacy aggregate command
// --------------------------------------------------------------------------

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
		fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
		os.Exit(1)
	}

	var files []string
	if info.IsDir() {
		walked, err := walkDir(pathArg, exts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		files = walked
	} else {
		files = []string{pathArg}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "⚠️  no files found")
		os.Exit(1)
	}

	for _, f := range files {
		if err := printFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "❌ error reading %s: %v\n", f, err)
		}
	}
}

// --------------------------------------------------------------------------
// index command
// --------------------------------------------------------------------------

func handleIndex(graphDB *string, args []string, verbose bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: codecontext index <path>")
		os.Exit(1)
	}
	dirPath := args[0]

	fmt.Printf("🔧 Opening database: %s\n", *graphDB)
	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	idx.SetVerbose(verbose)

	fmt.Printf("📁 Indexing directory: %s\n", dirPath)
	if err := idx.IndexDirectory(dirPath); err != nil {
		fmt.Fprintf(os.Stderr, "❌ error indexing directory: %v\n", err)
		os.Exit(1)
	}

	// Print final stats.
	stats, _ := idx.GetStats()
	fmt.Printf("✅ Indexing complete!\n")
	fmt.Printf("   📄 Files:        %v\n", stats["files"])
	fmt.Printf("   📝 LOC:          %v\n", formatCount(stats["lines_of_code"]))
	fmt.Printf("   🪙  Tokens:       %v\n", formatCount(stats["tokens"]))
	fmt.Printf("   🧩 Entities:     %v\n", stats["entities"])
	fmt.Printf("   🔗 Relations:    %v\n", stats["relations"])
	fmt.Printf("   📦 Dependencies: %v\n", stats["dependencies"])
	fmt.Printf("   💾 Database:     %s\n", *graphDB)
}

// --------------------------------------------------------------------------
// query command
// --------------------------------------------------------------------------

func handleQuery(graphDB *string, args []string, verbose bool) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: codecontext query <type> <query>")
		fmt.Fprintln(os.Stderr, "types: entity, calls, deps")
		os.Exit(1)
	}
	queryType := args[0]
	query := args[1]

	fmt.Printf("🔧 Opening database: %s\n", *graphDB)
	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	idx.SetVerbose(verbose)

	switch queryType {
	case "entity":
		fmt.Printf("🔍 Searching for entity: %q\n", query)
		entities, err := idx.QueryEntity(query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error querying entity: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("📋 Found %d entities:\n", len(entities))
		for _, e := range entities {
			fmt.Printf("  • [%s] %s  (ID=%d, file=%d, lines %d-%d)\n",
				e.Type, e.Name, e.ID, e.FileID, e.StartLine, e.EndLine)
			if e.Signature != "" {
				fmt.Printf("    Signature: %s\n", e.Signature)
			}
			if e.Parent != "" {
				fmt.Printf("    Parent:    %s\n", e.Parent)
			}
		}

	case "calls":
		var entityID int64
		fmt.Sscanf(query, "%d", &entityID)
		if entityID == 0 {
			fmt.Fprintf(os.Stderr, "❌ invalid entity ID: %s\n", query)
			os.Exit(1)
		}
		fmt.Printf("🔗 Call graph for entity %d:\n", entityID)
		graph, err := idx.QueryCallGraph(entityID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error querying call graph: %v\n", err)
			os.Exit(1)
		}
		printJSON(graph)

	case "deps":
		fmt.Printf("📦 Dependencies for: %s\n", query)
		graph, err := idx.QueryDependencyGraph(query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error querying dependencies: %v\n", err)
			os.Exit(1)
		}
		printJSON(graph)

	default:
		fmt.Fprintf(os.Stderr, "❌ unknown query type: %s\n", queryType)
		os.Exit(1)
	}
}

// --------------------------------------------------------------------------
// docs command
// --------------------------------------------------------------------------

func handleDocs(graphDB *string, args []string, verbose bool) {
	fs := flag.NewFlagSet("docs", flag.ExitOnError)
	useAI := fs.Bool("ai", false, "use AI to write entity descriptions")
	prompt := fs.String("prompt", "", "custom instruction for AI output style (use with -ai)")
	output := fs.String("output", "", "write documentation to this file (default: stdout)")
	_ = fs.Parse(args)

	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	idx.SetVerbose(verbose)

	var content string

	if *useAI {
		fmt.Fprintln(os.Stderr, "⚙️  Loading LLM configuration...")
		cfg := llm.LoadConfig()
		provider, err := llm.NewProvider(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error creating LLM provider: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()
		healthy, healthErr := provider.IsHealthy(ctx)
		if healthErr != nil || !healthy {
			msg := "provider did not respond"
			if healthErr != nil {
				msg = healthErr.Error()
			}
			fmt.Fprintf(os.Stderr, "❌ LLM provider not available: %s\n", msg)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "✅ Provider ready (%s / %s)\n", cfg.Provider, cfg.Model)

		chain := ai.NewChain(idx, provider)
		progress := func(path string) {
			fmt.Fprintf(os.Stderr, "  📝 %s\n", path)
		}
		content, err = chain.GenerateProjectDocs(ctx, *prompt, progress)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error generating docs: %v\n", err)
			os.Exit(1)
		}
	} else {
		content, err = standardDocs(idx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error generating docs: %v\n", err)
			os.Exit(1)
		}
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "❌ error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "✅ Documentation written to %s\n", *output)
	} else {
		fmt.Print(content)
	}
}

// standardDocs generates plain Markdown documentation from the indexed DB
// without any AI calls.  Entities are grouped by file; each entity gets a
// sub-section with its type, signature, visibility, and line range.
func standardDocs(idx *indexer.Indexer) (string, error) {
	files, err := idx.GetAllFiles()
	if err != nil {
		return "", fmt.Errorf("failed to get files: %w", err)
	}
	entities, err := idx.GetAllEntities()
	if err != nil {
		return "", fmt.Errorf("failed to get entities: %w", err)
	}
	stats, _ := idx.GetStats()

	byFile := make(map[int64][]*db.Entity)
	for _, e := range entities {
		byFile[e.FileID] = append(byFile[e.FileID], e)
	}

	var sb strings.Builder
	sb.WriteString("# Project Documentation\n\n")
	sb.WriteString(fmt.Sprintf(
		"_Generated from indexed database — %v files, %v entities_\n\n---\n\n",
		stats["files"], stats["entities"],
	))

	for _, f := range files {
		ents := byFile[f.ID]
		if len(ents) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("## `%s` · %s\n\n", f.Path, f.Language))

		for _, e := range ents {
			// Heading: qualified name when entity has a parent.
			heading := fmt.Sprintf("`%s`", e.Name)
			if e.Parent != "" {
				heading = fmt.Sprintf("`%s.%s`", e.Parent, e.Name)
			}
			sb.WriteString(fmt.Sprintf("### %s · %s · lines %d–%d\n\n", heading, e.Type, e.StartLine, e.EndLine))

			if e.Signature != "" {
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", e.Signature))
			}
			if e.Visibility != "" {
				sb.WriteString(fmt.Sprintf("- **Visibility:** %s\n", e.Visibility))
			}
			if e.Kind != "" && e.Kind != e.Type {
				sb.WriteString(fmt.Sprintf("- **Kind:** %s\n", e.Kind))
			}
			if e.Documentation != "" {
				sb.WriteString("\n" + e.Documentation + "\n")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}

	return sb.String(), nil
}

// --------------------------------------------------------------------------
// stats command
// --------------------------------------------------------------------------

func handleStats(graphDB *string, verbose bool) {
	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	idx.SetVerbose(verbose)
	stats, err := idx.GetStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error getting stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("📊 Code Graph Statistics:")
	fmt.Printf("   📄 Files:        %v\n", stats["files"])
	fmt.Printf("   📝 LOC:          %v\n", formatCount(stats["lines_of_code"]))
	fmt.Printf("   🪙  Tokens:       %v\n", formatCount(stats["tokens"]))
	fmt.Printf("   🧩 Entities:     %v\n", stats["entities"])
	fmt.Printf("   📦 Dependencies: %v\n", stats["dependencies"])
	fmt.Printf("   🔗 Relations:    %v\n", stats["relations"])
}

// --------------------------------------------------------------------------
// web command
// --------------------------------------------------------------------------

func handleWeb(graphDB *string, args []string, verbose bool) {
	port := "8080"
	if len(args) > 0 {
		port = args[0]
	}

	fmt.Printf("🔧 Opening database: %s\n", *graphDB)
	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	idx.SetVerbose(verbose)

	srv := web.New(idx)
	fmt.Printf("🌐 Starting graph UI at http://localhost:%s\n", port)
	fmt.Println("   Press Ctrl+C to stop")
	if err := srv.Listen(":" + port); err != nil {
		fmt.Fprintf(os.Stderr, "❌ web server error: %v\n", err)
		os.Exit(1)
	}
}

// --------------------------------------------------------------------------
// mcp command
// --------------------------------------------------------------------------

func handleMCP(graphDB *string, args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	useHTTP := fs.Bool("http", false, "serve over HTTP instead of stdio")
	addr := fs.String("addr", ":8081", "HTTP listen address (only with -http)")
	_ = fs.Parse(args)

	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	idx := indexer.New(database)
	mcpServer := ccmcp.New(idx, version)

	if *useHTTP {
		fmt.Fprintf(os.Stderr, "🔌 MCP HTTP server listening on http://localhost%s\n", *addr)
		fmt.Fprintf(os.Stderr, "   POST /mcp        Streamable HTTP endpoint\n")
		if err := mcpServer.ListenHTTP(*addr); err != nil {
			fmt.Fprintf(os.Stderr, "❌ MCP HTTP server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Fprintln(os.Stderr, `{"level":"INFO","msg":"mcp stdio server ready","db":"`+*graphDB+`"}`)

	if err := mcpServer.Inner().Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "❌ MCP stdio server error: %v\n", err)
		os.Exit(1)
	}
}

// --------------------------------------------------------------------------
// ai command
// --------------------------------------------------------------------------

func handleAI(graphDB *string, args []string, verbose bool) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: codecontext ai <subcommand> [args]")
		fmt.Fprintln(os.Stderr, "subcommands: query, analyze, docs, review, summarize, chat")
		os.Exit(1)
	}

	subcommand := args[0]
	subargs := args[1:]

	fmt.Printf("🔧 Opening database: %s\n", *graphDB)
	database, err := db.Open(*graphDB, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	fmt.Println("⚙️  Loading LLM configuration...")
	cfg := llm.LoadConfig()
	if verbose {
		fmt.Printf("   Provider:    %s\n", cfg.Provider)
		fmt.Printf("   Model:       %s\n", cfg.Model)
		fmt.Printf("   Temperature: %.2f\n", cfg.Temperature)
		fmt.Printf("   MaxTokens:   %d\n", cfg.MaxTokens)
		if cfg.Provider == llm.ProviderAzure {
			fmt.Printf("   Endpoint:    %s\n", cfg.AzureOpenAIEndpoint)
			fmt.Printf("   API version: %s\n", cfg.AzureOpenAIVersion)
		}
	}

	fmt.Printf("🔌 Creating %s provider...\n", cfg.Provider)
	provider, err := llm.NewProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ error creating LLM provider: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	fmt.Printf("🏥 Checking provider health...\n")
	healthy, healthErr := provider.IsHealthy(ctx)
	if healthErr != nil || !healthy {
		msg := "provider did not respond"
		if healthErr != nil {
			msg = healthErr.Error()
		}
		fmt.Fprintf(os.Stderr, "❌ LLM provider is not available: %s\n", msg)
		fmt.Fprintln(os.Stderr, "   Verify your environment variables:")
		switch cfg.Provider {
		case llm.ProviderAzure:
			fmt.Fprintln(os.Stderr, "   AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_KEY, AZURE_OPENAI_DEPLOYMENT, AZURE_OPENAI_API_VERSION")
		case llm.ProviderOpenAI:
			fmt.Fprintln(os.Stderr, "   OPENAI_API_KEY, OPENAI_MODEL")
		case llm.ProviderOllama:
			fmt.Fprintln(os.Stderr, "   OLLAMA_BASE_URL (default http://localhost:11434), LLM_MODEL")
		}
		os.Exit(1)
	}
	fmt.Printf("✅ Provider healthy (%s / %s)\n", cfg.Provider, cfg.Model)

	idx := indexer.New(database)
	idx.SetVerbose(verbose)
	chain := ai.NewChain(idx, provider)

	switch subcommand {
	case "query":
		if len(subargs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: codecontext ai query <question>")
			os.Exit(1)
		}
		question := strings.Join(subargs, " ")
		fmt.Printf("💬 Querying: %q\n\n", question)
		response, err := chain.QueryNatural(ctx, question)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "analyze":
		if len(subargs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: codecontext ai analyze <entity_name>")
			os.Exit(1)
		}
		fmt.Printf("🔬 Analysing entity: %s\n\n", subargs[0])
		response, err := chain.AnalyzeEntity(ctx, subargs[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "docs":
		if len(subargs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: codecontext ai docs <entity_name>")
			os.Exit(1)
		}
		fmt.Printf("📝 Generating docs for: %s\n\n", subargs[0])
		response, err := chain.GenerateDocs(ctx, subargs[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "review":
		if len(subargs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: codecontext ai review <entity_name>")
			os.Exit(1)
		}
		fmt.Printf("👀 Reviewing: %s\n\n", subargs[0])
		response, err := chain.ReviewCode(ctx, subargs[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "summarize":
		if len(subargs) == 0 {
			fmt.Fprintln(os.Stderr, "usage: codecontext ai summarize <file_path>")
			os.Exit(1)
		}
		fmt.Printf("📄 Summarising: %s\n\n", subargs[0])
		response, err := chain.Summarize(ctx, subargs[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)

	case "chat":
		handleAIChat(ctx, chain)

	default:
		fmt.Fprintf(os.Stderr, "❌ unknown AI subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func handleAIChat(ctx context.Context, chain *ai.Chain) {
	fmt.Println("💬 AI Chat — interactive conversation about code")
	fmt.Println("   Commands: analyze <entity>  docs <entity>  review <entity>  exit")
	fmt.Println()

	conversation := &ai.ConversationContext{
		Messages: []*ai.Message{
			{Role: "system", Content: "You are a helpful code analysis assistant. You have access to a codebase and can help analyze, explain, and improve code."},
		},
	}

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		input := scanner.Text()
		if input == "exit" {
			fmt.Println("👋 Goodbye!")
			return
		}

		parts := strings.Fields(input)
		handled := false
		if len(parts) > 0 {
			switch parts[0] {
			case "analyze", "docs", "review":
				if len(parts) < 2 {
					fmt.Printf("usage: %s <entity_name>\n", parts[0])
					handled = true
				} else {
					var (
						response string
						err      error
					)
					switch parts[0] {
					case "analyze":
						response, err = chain.AnalyzeEntity(ctx, parts[1])
					case "docs":
						response, err = chain.GenerateDocs(ctx, parts[1])
					case "review":
						response, err = chain.ReviewCode(ctx, parts[1])
					}
					if err != nil {
						fmt.Printf("❌ Error: %v\n", err)
					} else {
						fmt.Println(response)
					}
					handled = true
				}
			}
		}

		if !handled {
			response, err := chain.Chat(ctx, conversation, input)
			if err != nil {
				fmt.Printf("❌ Error: %v\n", err)
			} else {
				fmt.Println(response)
			}
		}
		fmt.Print("> ")
	}
}

// --------------------------------------------------------------------------
// Utilities
// --------------------------------------------------------------------------

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

func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func formatCount(v interface{}) string {
	var n int64
	switch val := v.(type) {
	case int:
		n = int64(val)
	case int32:
		n = int64(val)
	case int64:
		n = val
	case float64:
		n = int64(val)
	default:
		return fmt.Sprintf("%v", v)
	}

	format := func(val float64, suffix string) string {
		s := fmt.Sprintf("%.1f", val)
		s = strings.TrimSuffix(s, ".0")
		return fmt.Sprintf("%d (%s%s)", n, s, suffix)
	}

	if n >= 1_000_000_000 {
		return format(float64(n)/1_000_000_000.0, "B")
	}
	if n >= 1_000_000 {
		return format(float64(n)/1_000_000.0, "M")
	}
	if n >= 1_000 {
		return format(float64(n)/1_000.0, "K")
	}
	return fmt.Sprintf("%d", n)
}
