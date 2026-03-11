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

# Start MCP server (HTTP)
./codecontext mcp -http -addr :8081

# Start web UI
./codecontext web -addr :8080
```

## Architecture

```
main.go                          CLI entry point, command routing (~800 lines)
pkg/
├── db/
│   ├── db.go                    GORM/SQLite operations (Open, Insert*, Get*, Delete*)
│   └── models.go                Schema: File, Entity, Dependency, EntityRelation
├── parser/
│   ├── parser.go                Language router + convert*ParseResult adapters
│   ├── parser_ast_test.go       Parser tests (Python, JS, Java)
│   ├── go/parser.go             Go: stdlib go/ast
│   ├── python/parser.go         Python: indentation-aware line scanner
│   ├── javascript/parser.go     JS/TS: two-pass tokenizer + brace-depth
│   └── java/parser.go           Java: two-pass tokenizer + brace-depth
├── indexer/
│   └── indexer.go               Parallel file indexer with worker pool
├── mcp/
│   └── server.go                MCP server (stdio + HTTP transport)
├── web/
│   ├── server.go                Web server (/api/graph, /api/stats)
│   └── ui.go                    Embedded SPA (HTML/CSS/JS in Go const)
├── llm/                         LLM provider abstraction (Ollama, Azure, OpenAI)
└── ai/                          AI analysis chains (query, analyze, docs, review)
```

## Key Patterns

### Database
- **GORM + SQLite** with auto-migration. No manual schema files.
- `Database.WithTx(tx)` returns a shallow copy for transactional writes.
- WAL journal mode is enabled for concurrent read/write performance.
- Foreign keys with CASCADE deletes — deleting a File cleans up its entities and deps.
- `FirstOrCreate` pattern prevents duplicate inserts (entities, deps, relations).

### Parsers
- Each language has its own package under `pkg/parser/{lang}/` with its own types.
- `pkg/parser/parser.go` routes by file extension and converts sub-parser results to the common `parser.ParseResult` type via `convert*ParseResult` functions.
- **Go parser** uses stdlib `go/ast` — the most accurate parser.
- **Python/JS/Java parsers** are hand-written scanners (no AST libraries). They work by:
  1. Stripping string literals and comments (preserving line structure)
  2. Matching keywords line-by-line with scope tracking (indent for Python, braces for JS/Java)
- Parsers extract: entities (functions, methods, classes, types), dependencies (imports), and parent-child relationships.

### Indexer
- Worker pool with `min(8, numCPU)` goroutines.
- A global `sync.Mutex` serializes DB writes (SQLite limitation).
- All writes per file are wrapped in a single GORM transaction.
- File change detection via MD5 hash — unchanged files are skipped on re-index.
- Respects `.gitignore` and `.ignore` files at any directory level.

### MCP Server
- **Stdio transport**: line-delimited JSON-RPC 2.0 on stdin/stdout (default).
- **HTTP transport**: `POST /mcp` for JSON-RPC, `GET /mcp/tools` for tool listing.
- Protocol version: `2025-03-26` (Streamable HTTP).
- The `Dispatch()` method is transport-agnostic — both transports call it.
- Error responses map JSON-RPC codes to HTTP status codes (400, 404, 500).

### Web UI
- Entirely self-contained — the whole SPA is a Go string constant in `pkg/web/ui.go`.
- No external JS/CSS dependencies. Pure SVG + vanilla JS force-directed graph.
- Retained-mode SVG rendering: elements are created once, positions updated per tick.
- `/api/graph` returns `{nodes, edges}` JSON; `/api/stats` returns counts.

## Code Conventions

- **Go version**: 1.22+ (uses `go/ast`, no generics in project code)
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
1. Create `pkg/parser/{lang}/parser.go` with its own types (`ParseResult`, `Entity`, `Dependency`).
2. Add a `convert{Lang}ParseResult` function in `pkg/parser/parser.go`.
3. Add the language constant and extension mapping in `detectLanguage()`.
4. Add the extension(s) to `isSourceFile()` in `pkg/indexer/indexer.go`.
5. Add tests in `pkg/parser/parser_ast_test.go`.

### Adding a new MCP tool
1. Add the tool definition to `GetTools()` in `pkg/mcp/server.go`.
2. Add the method case to `dispatch()` and `CallTool()`.
3. Implement the handler method on `*Server`.

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
- **Python/JS/Java parsers are heuristic**: They're line-based scanners, not full AST parsers. Edge cases (deeply nested closures, complex destructuring) may be missed.
- **SQLite single-writer**: Despite the worker pool, all DB writes go through a single mutex. WAL mode helps reads but writes are still serialized.
- **Web UI performance**: O(n^2) repulsion is skipped for graphs > 400 nodes. Large graphs (800+) hide entity nodes by default.

## File Dependencies (what touches what)

```
main.go → all pkg/* packages
pkg/indexer → pkg/parser, pkg/db
pkg/mcp     → pkg/indexer (queries only)
pkg/web     → pkg/indexer (queries only)
pkg/ai      → pkg/llm, pkg/indexer
pkg/parser  → pkg/parser/{go,python,javascript,java}
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
