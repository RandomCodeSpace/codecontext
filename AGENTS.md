# AGENTS.md — Universal AI Agent Instructions for codecontext

> This file is intended for any AI coding agent (Claude Code, Cursor, Copilot, Windsurf, Aider, Cody, Continue, etc.) working on this codebase. For Claude-specific details, also see `CLAUDE.md`.

## What Is This Project?

**codecontext** is a Go CLI tool that:
1. **Indexes** source code (Go, Python, JS/TS, Java) into a SQLite-backed code graph
2. **Exposes** the graph via MCP (Model Context Protocol), a web UI, and AI analysis
3. **Parses** code using tree-sitter (Python, JS/TS, Java) and Go's stdlib `go/ast` (Go)

## Essential Commands

```bash
# Build the binary
go build -o codecontext

# Run ALL tests (do this before any commit)
go test ./...

# Run parser tests only
go test ./pkg/parser/ -v

# Lint
go vet ./...

# Smoke test: index this project and check stats
./codecontext index .
./codecontext stats

# Start MCP server (stdio mode — default)
./codecontext mcp

# Start MCP server (HTTP mode, Streamable HTTP transport)
./codecontext mcp -http -addr :8081

# Start web UI
./codecontext web -addr :8080
```

## Project Structure

```
main.go                             CLI entry point, command routing
pkg/
├── db/
│   ├── db.go                       GORM/SQLite CRUD operations
│   └── models.go                   Schema: File, Entity, Dependency, EntityRelation
├── parser/
│   ├── parser.go                   Language router + convert*ParseResult adapters
│   ├── types.go                    Common types: ParseResult, Entity, Dependency, Language
│   ├── parser_ast_test.go          Tests for Python, JS, Java parsers
│   ├── go/parser.go                Go parser (stdlib go/ast)
│   ├── python/parser.go            Python parser (tree-sitter)
│   ├── javascript/parser.go        JS/TS parser (tree-sitter)
│   └── java/parser.go              Java parser (tree-sitter)
├── indexer/
│   └── indexer.go                  Parallel file indexer with worker pool
├── mcp/
│   └── server.go                   MCP server (official Go SDK)
├── web/
│   ├── server.go                   Web server (/api/graph, /api/stats)
│   └── ui.go                       Self-contained SPA (embedded HTML/CSS/JS)
├── llm/                            LLM providers (Ollama, Azure, OpenAI)
└── ai/                             AI analysis chains
.github/workflows/
├── test-cli.yml                    CI: build + integration tests
└── release.yml                     Release pipeline
```

## Key Dependencies

| Package | Purpose |
|---|---|
| `github.com/smacker/go-tree-sitter` | Tree-sitter Go bindings (Python, JS, Java grammars) |
| `github.com/modelcontextprotocol/go-sdk/mcp` | Official MCP Go SDK (aliased as `mcpsdk`) |
| `gorm.io/gorm` + `gorm.io/driver/sqlite` | ORM + SQLite |
| `github.com/sashabaranov/go-openai` | OpenAI API client |
| `github.com/sabhiram/go-gitignore` | .gitignore pattern matching |

## Coding Rules

1. **Go 1.24+** — uses generics via MCP SDK
2. **Error handling** — always wrap: `fmt.Errorf("context: %w", err)`
3. **Logging** — `log/slog` JSON to stderr (MCP server), `fmt.Printf` for CLI
4. **Testing** — stdlib `testing` only, no external frameworks
5. **Commits** — `type(scope): message` format (e.g., `feat(parser): add Ruby support`)
6. **Keep it simple** — no over-engineering, no premature abstractions
7. **Security** — no command injection, validate external input at boundaries

## How Things Work

### Parsers
- `pkg/parser/parser.go` routes files by extension to the correct sub-parser
- Go parser uses stdlib `go/ast` (most accurate)
- Python/JS/Java use **tree-sitter**: `sitter.NewParser()` → set language → `ParseCtx()` → walk AST
- Each sub-parser returns its own `ParseResult` type; `convert*ParseResult` adapters normalize them
- Tree-sitter nodes: `node.NamedChild(i)`, `node.ChildByFieldName("name")`, `node.Content(src)`
- Line numbers: `node.StartPoint().Row + 1` (tree-sitter is 0-indexed)

### Database
- GORM auto-migration on startup (no manual schema files)
- `FirstOrCreate` prevents duplicate entities/deps/relations
- `WithTx(tx)` returns a shallow copy for transactional writes
- WAL mode for concurrent reads; single mutex for writes
- CASCADE deletes: removing a File removes its entities and deps

### MCP Server
- Wraps `*mcpsdk.Server` in a `Server` struct with `Inner()` accessor
- Tools registered via `mcpsdk.AddTool(server, tool, handler)` with typed input structs
- Input schemas auto-generated from `json`/`jsonschema` struct tags
- Stdio: `mcpServer.Inner().Run(ctx, &mcpsdk.StdioTransport{})`
- HTTP: `mcpsdk.NewStreamableHTTPHandler(getServer, nil)` (Streamable HTTP, MCP 2025-03-26+)
- Tools: `index_directory`, `query_entity`, `query_call_graph`, `query_dependencies`, `graph_stats`, `get_docs`

### Indexer
- Worker pool: `min(8, numCPU)` goroutines
- File change detection via MD5 hash — skips unchanged files
- All writes per file wrapped in a single GORM transaction
- Respects `.gitignore` and `.ignore` at any directory level

### Web UI
- Entire SPA is a Go string constant in `pkg/web/ui.go`
- No external dependencies — pure SVG + vanilla JS
- `/api/graph` returns `{nodes, edges}`; `/api/stats` returns counts

## Common Development Tasks

### Add a new parser language
1. `go get github.com/smacker/go-tree-sitter/{lang}`
2. Create `pkg/parser/{lang}/parser.go` with tree-sitter setup
3. Define `ParseResult`, `Entity`, `Dependency` types in that package
4. Add `convert{Lang}ParseResult` in `pkg/parser/parser.go`
5. Add language constant + extension mapping in `detectLanguage()`
6. Add extension to `isSourceFile()` in `pkg/indexer/indexer.go`
7. Add tests in `pkg/parser/parser_ast_test.go`
8. Run `go test ./pkg/parser/ -v`

### Add a new MCP tool
1. Define input args struct with `json` + `jsonschema` tags in `pkg/mcp/server.go`
2. Register via `mcpsdk.AddTool(s.inner, &mcpsdk.Tool{...}, s.handler)` in `registerTools()`
3. Implement handler: `func(ctx, *mcpsdk.CallToolRequest, ArgsType) (*mcpsdk.CallToolResult, any, error)`
4. Return via `textResult(serializeJSON(data))`

### Modify the database schema
1. Edit models in `pkg/db/models.go` (GORM auto-migrates)
2. Add query methods in `pkg/db/db.go` if needed
3. Update `pkg/indexer/indexer.go` if the indexer populates the new field
4. Update `pkg/mcp/server.go` if MCP needs to expose it

### Modify the web UI
- Edit the HTML/CSS/JS string in `pkg/web/ui.go`
- Key functions: `rebuildSVG()` (creates elements), `renderGraph()` (updates positions)
- `buildSimData()` filters visible nodes/edges, `initSimulation()` starts physics

## File Dependency Map

```
main.go → all pkg/*, github.com/modelcontextprotocol/go-sdk/mcp
pkg/indexer → pkg/parser, pkg/db
pkg/mcp     → pkg/db, pkg/indexer, github.com/modelcontextprotocol/go-sdk/mcp
pkg/web     → pkg/indexer
pkg/ai      → pkg/llm, pkg/indexer
pkg/parser  → pkg/parser/{go,python,javascript,java}
pkg/parser/{python,javascript,java} → github.com/smacker/go-tree-sitter
pkg/parser/go → stdlib go/ast
pkg/db      → gorm, sqlite
```

## Environment Variables (AI features only)

```bash
LLM_PROVIDER=ollama|azure|openai
OLLAMA_BASE_URL=http://localhost:11434
OPENAI_API_KEY=sk-...
AZURE_OPENAI_ENDPOINT=https://...
AZURE_OPENAI_KEY=...
AZURE_OPENAI_DEPLOYMENT=...
```

Config precedence: `.env.local` > `.env` > environment > defaults.

## Known Limitations
- No call graph analysis — only parent→child "defines" relations
- TypeScript uses the JavaScript tree-sitter grammar (complex TS syntax may be missed)
- SQLite single-writer despite worker pool (WAL helps reads, writes serialized via mutex)
- Web UI skips O(n^2) repulsion for graphs > 400 nodes; hides entities for 800+ nodes
