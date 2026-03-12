# CLAUDE.md — AI Agent Instructions for codecontext

This file provides context and instructions for AI agents (Claude Code, Copilot, Cursor, etc.) working on this codebase.

## Project Overview

**codecontext** is a Go CLI tool that indexes source code into a SQLite-backed code graph and exposes it via MCP (Model Context Protocol), a web UI, and AI-powered analysis. It parses Go, Python, JavaScript/TypeScript, and Java.

## Quick Reference

```bash
# Build
go build -o codecontext

# Run all tests
go test ./...

# Run a specific test
go test ./pkg/parser/ -run TestPythonParser -v

# Lint (no external linter configured — use go vet)
go vet ./...

# Index a project (manual smoke test)
./codecontext index .
./codecontext stats

# Start MCP server (stdio)
./codecontext mcp

# Start MCP server (HTTP, Streamable HTTP transport)
./codecontext mcp -http -addr :8081

# Start web UI
./codecontext web -addr :8080
```

## Architecture

```
main.go                          CLI entry point, command routing
pkg/
├── db/
│   ├── db.go                    GORM/SQLite operations (Open, Insert*, Get*, Delete*)
│   └── models.go                Schema: File, Entity, Dependency, EntityRelation
├── parser/
│   ├── parser.go                Language router + convert*ParseResult adapters
│   ├── types.go                 Common types: ParseResult, Entity, Dependency, Language
│   ├── parser_ast_test.go       Parser tests (Python, JS, Java)
│   ├── go/parser.go             Go: stdlib go/ast
│   ├── python/parser.go         Python: tree-sitter AST
│   ├── javascript/parser.go     JS/TS: tree-sitter AST
│   └── java/parser.go           Java: tree-sitter AST
├── indexer/
│   └── indexer.go               Parallel file indexer with worker pool
├── mcp/
│   └── server.go                MCP server (official Go SDK, stdio + Streamable HTTP)
├── web/
│   ├── server.go                Web server (/api/graph, /api/stats)
│   └── ui.go                    Embedded SPA (HTML/CSS/JS in Go const)
├── llm/                         LLM provider abstraction (Ollama, Azure, OpenAI)
└── ai/                          AI analysis chains (query, analyze, docs, review)
```

## Key Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/smacker/go-tree-sitter` | Tree-sitter Go bindings for Python, JS/TS, Java parsing |
| `github.com/modelcontextprotocol/go-sdk/mcp` | Official MCP Go SDK (server, tools, transports) |
| `gorm.io/gorm` + `gorm.io/driver/sqlite` | ORM + SQLite database |
| `github.com/sashabaranov/go-openai` | OpenAI API client (for AI features) |
| `github.com/sabhiram/go-gitignore` | .gitignore pattern matching |

## Key Patterns

### Database
- **GORM + SQLite** with auto-migration. No manual schema files.
- `Database.WithTx(tx)` returns a shallow copy for transactional writes.
- WAL journal mode is enabled for concurrent read/write performance.
- Foreign keys with CASCADE deletes — deleting a File cleans up its entities and deps.
- `FirstOrCreate` pattern prevents duplicate inserts (entities, deps, relations).

### Parsers (tree-sitter)
- Each language has its own package under `pkg/parser/{lang}/` with its own types.
- `pkg/parser/parser.go` routes by file extension and converts sub-parser results to the common `parser.ParseResult` type via `convert*ParseResult` functions.
- **Go parser** uses stdlib `go/ast` — the most accurate parser.
- **Python, JavaScript/TypeScript, and Java parsers** use **tree-sitter** (`github.com/smacker/go-tree-sitter`):
  - Each parser creates a `sitter.NewParser()`, sets the language grammar, and calls `parser.ParseCtx()`.
  - AST walking is done via `node.NamedChild(i)` and `node.ChildByFieldName("name")`.
  - Node text is extracted via `node.Content(src)` where `src` is the `[]byte` source.
  - Line numbers use `node.StartPoint().Row + 1` (tree-sitter is 0-indexed).
- Parsers extract: entities (functions, methods, classes, types), dependencies (imports), and parent-child relationships.
- Tree-sitter grammars used:
  - `github.com/smacker/go-tree-sitter/python` — Python
  - `github.com/smacker/go-tree-sitter/javascript` — JavaScript (also used for TypeScript)
  - `github.com/smacker/go-tree-sitter/java` — Java

### Indexer
- Worker pool with `min(8, numCPU)` goroutines.
- A global `sync.Mutex` serializes DB writes (SQLite limitation).
- All writes per file are wrapped in a single GORM transaction.
- File change detection via MD5 hash — unchanged files are skipped on re-index.
- Respects `.gitignore` and `.ignore` files at any directory level.

### MCP Server (official Go SDK)
- Uses `github.com/modelcontextprotocol/go-sdk/mcp` (aliased as `mcpsdk` in code).
- `Server` struct wraps `*mcpsdk.Server` and exposes `Inner()` for transport access.
- Tools are registered via `mcpsdk.AddTool(server, &mcpsdk.Tool{...}, handler)` with typed input structs.
  - Input schemas are **auto-generated** from Go struct tags: `json:"name" jsonschema:"description"`.
  - Handler signature: `func(ctx, *mcpsdk.CallToolRequest, ArgsStruct) (*mcpsdk.CallToolResult, any, error)`.
- **Stdio transport**: `mcpServer.Inner().Run(ctx, &mcpsdk.StdioTransport{})` — handles all JSON-RPC.
- **HTTP transport**: `mcpsdk.NewStreamableHTTPHandler(getServer, nil)` — Streamable HTTP (MCP 2025-03-26+).
- Six tools registered: `index_directory`, `query_entity`, `query_call_graph`, `query_dependencies`, `graph_stats`, `get_docs`.

### Web UI
- Entirely self-contained — the whole SPA is a Go string constant in `pkg/web/ui.go`.
- No external JS/CSS dependencies. Pure SVG + vanilla JS force-directed graph.
- Retained-mode SVG rendering: elements are created once, positions updated per tick.
- `/api/graph` returns `{nodes, edges}` JSON; `/api/stats` returns counts.

## Code Conventions

- **Go version**: 1.24+ (uses generics in MCP SDK integration)
- **Error handling**: Return `fmt.Errorf("context: %w", err)` with wrapping.
- **Logging**: `log/slog` JSON handler to stderr (MCP server). `fmt.Printf` for CLI output.
- **No external test framework** — stdlib `testing` only.
- **Emoji in CLI output** (progress, errors) is intentional for user-facing messages.
- **Commit style**: `type(scope): message` — e.g., `fix(mcp): fix HTTP error codes`

## Testing

Tests exist for:
- `main_test.go` — basic file aggregation and directory walking
- `pkg/parser/parser_ast_test.go` — Python, JavaScript, Java parser correctness

CI runs via `.github/workflows/test-cli.yml` which builds, indexes sample projects (Go, Python, JS, Java, mixed), and verifies entity queries work end-to-end.

When modifying parsers, always run:
```bash
go test ./pkg/parser/ -v
```

When modifying any package, run the full suite:
```bash
go test ./...
```

## Common Tasks

### Adding a new parser language
1. `go get github.com/smacker/go-tree-sitter/{lang}` to add the tree-sitter grammar.
2. Create `pkg/parser/{lang}/parser.go` — create a `sitter.NewParser()`, set the language, walk the AST.
3. Define the same output types (`ParseResult`, `Entity`, `Dependency`) in that package.
4. Add a `convert{Lang}ParseResult` function in `pkg/parser/parser.go`.
5. Add the language constant and extension mapping in `detectLanguage()` in `pkg/parser/parser.go`.
6. Add the extension(s) to `isSourceFile()` in `pkg/indexer/indexer.go`.
7. Add tests in `pkg/parser/parser_ast_test.go`.

### Adding a new MCP tool
1. Define an input args struct with `json` and `jsonschema` tags in `pkg/mcp/server.go`.
2. Register the tool via `mcpsdk.AddTool(s.inner, &mcpsdk.Tool{Name: "...", Description: "..."}, s.handler)` in `registerTools()`.
3. Implement the handler method on `*Server` with signature: `func(ctx, *mcpsdk.CallToolRequest, ArgsType) (*mcpsdk.CallToolResult, any, error)`.
4. Return results via `textResult(serializeJSON(data))` helper.

### Modifying the web UI
- Edit the HTML/CSS/JS string in `pkg/web/ui.go`.
- The `rebuildSVG()` function creates DOM elements; `renderGraph()` updates positions.
- `buildSimData()` filters visible nodes/edges; `initSimulation()` starts physics.

### Modifying the database schema
- Edit models in `pkg/db/models.go`. GORM auto-migrates on startup.
- If adding query methods, add them to `pkg/db/db.go`.
- If the indexer needs the new field, update `pkg/indexer/indexer.go`.
- If MCP needs to expose it, update `pkg/mcp/server.go`.

## Known Limitations

- **No call graph analysis**: Only "defines" relations (parent→child) are built. Actual function call detection is not implemented.
- **TypeScript uses JavaScript grammar**: The JS tree-sitter parser handles basic TS, but complex TS-specific syntax (decorators, advanced generics) may not be fully captured. Consider adding `github.com/smacker/go-tree-sitter/typescript/typescript` for full TS support.
- **SQLite single-writer**: Despite the worker pool, all DB writes go through a single mutex. WAL mode helps reads but writes are still serialized.
- **Web UI performance**: O(n^2) repulsion is skipped for graphs > 400 nodes. Large graphs (800+) hide entity nodes by default.

## File Dependencies (what touches what)

```
main.go → all pkg/* packages, github.com/modelcontextprotocol/go-sdk/mcp
pkg/indexer → pkg/parser, pkg/db
pkg/mcp     → pkg/db, pkg/indexer, github.com/modelcontextprotocol/go-sdk/mcp
pkg/web     → pkg/indexer (queries only)
pkg/ai      → pkg/llm, pkg/indexer
pkg/parser  → pkg/parser/{go,python,javascript,java}
pkg/parser/{python,javascript,java} → github.com/smacker/go-tree-sitter
pkg/parser/go → stdlib go/ast (no external deps)
pkg/db      → gorm, sqlite (no internal deps)
```

## Environment Variables (for AI features)

```bash
LLM_PROVIDER=ollama|azure|openai    # Required for `ai` commands
OLLAMA_BASE_URL=http://localhost:11434
OPENAI_API_KEY=sk-...
AZURE_OPENAI_ENDPOINT=https://...
AZURE_OPENAI_KEY=...
AZURE_OPENAI_DEPLOYMENT=...
```

Config loaded from: `.env.local` > `.env` > environment > defaults.
