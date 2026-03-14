# AGENTS.md — Universal AI Agent Instructions for codecontext

> Canonical instruction source: this file is the source of truth for agent behavior and project guidance in this repository.

> This file is intended for any AI coding agent (Claude Code, Cursor, Copilot, Windsurf, Aider, Cody, Continue, etc.) working on this codebase.

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

> Note: Sections below were merged from removed files to preserve all prior guidance in one place.


---

## Merged Content from copilot-instructions.md

# GitHub Copilot Instructions — codecontext

> Instruction authority: use `.github/AGENTS.md` as the canonical guide for all agents. Use `.github/CLAUDE.md` only for Claude-specific details. If content conflicts, prefer `.github/AGENTS.md`.

## Project Overview
codecontext is a Go CLI tool that indexes source code into a SQLite-backed code graph and exposes it via MCP (Model Context Protocol), a web UI, and AI-powered analysis. It parses Go, Python, JavaScript/TypeScript, and Java using tree-sitter.

## Build & Test Commands
```bash
go build -o codecontext          # Build
go test ./...                    # Run all tests
go test ./pkg/parser/ -v         # Test parsers specifically
go vet ./...                     # Lint
```

## Code Conventions
- **Go 1.24+**, stdlib `testing` only
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` JSON handler to stderr for MCP server; `fmt.Printf` for CLI output
- Commit messages: `type(scope): message` (e.g., `fix(mcp): handle nil pointer`)
- Keep it simple — no over-engineering, no unnecessary abstractions

## Architecture Summary
| Package | Role |
|---|---|
| `main.go` | CLI entry point |
| `pkg/db` | GORM + SQLite (models + operations) |
| `pkg/parser` | Language router + per-language sub-packages |
| `pkg/parser/go` | Go parser (stdlib `go/ast`) |
| `pkg/parser/{python,javascript,java}` | Tree-sitter based parsers |
| `pkg/indexer` | Parallel file indexer, mutex-serialized writes |
| `pkg/mcp` | MCP server (official Go SDK, stdio + HTTP) |
| `pkg/web` | Self-contained SPA embedded in Go |
| `pkg/llm` | LLM provider abstraction |
| `pkg/ai` | AI analysis chains |

## Key Dependencies
- `github.com/smacker/go-tree-sitter` — tree-sitter bindings for Python/JS/Java
- `github.com/modelcontextprotocol/go-sdk/mcp` — official MCP Go SDK
- `gorm.io/gorm` + `gorm.io/driver/sqlite` — ORM + database
- `github.com/sashabaranov/go-openai` — OpenAI client

## Important Patterns
- Tree-sitter parsers create `sitter.NewParser()`, set language, call `parser.ParseCtx()`, walk AST
- MCP tools use `mcpsdk.AddTool()` with typed structs (`json`/`jsonschema` tags)
- DB uses `FirstOrCreate` to prevent duplicates, `WithTx()` for transactions
- Indexer detects changes via MD5 hash, respects `.gitignore`

## Testing Requirements
- Always run `go test ./pkg/parser/ -v` after parser changes
- Always run `go test ./...` after any package change
- CI: `.github/workflows/test-cli.yml` — builds and indexes sample projects

## For Detailed Instructions
See `.github/AGENTS.md` for canonical instructions and `.github/CLAUDE.md` for Claude-specific additions.


---

## Merged Content from CLAUDE.md

# CLAUDE.md — AI Agent Instructions for codecontext

> Reference priority: follow `.github/AGENTS.md` for shared project instructions. This file should only add Claude-specific guidance.

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


---

## Merged Content from AI_FEATURES.md

# AI-Powered Code Analysis Features

> Instruction note: this document is feature reference only. For coding-agent behavior and project instructions, use `.github/AGENTS.md` (canonical) and `.github/CLAUDE.md` (Claude-specific).

This document describes the AI-powered code analysis capabilities of codecontext, built on a multi-provider LLM abstraction layer.

## Architecture

The AI system is built in three layers:

### 1. LLM Provider Layer (`pkg/llm/`)

The provider layer abstracts LLM interactions across multiple services:

- **Provider Interface**: Defines common methods for all LLM providers
  - `Complete()`: Text completion
  - `Chat()`: Multi-turn conversation
  - `GetModel()`: Model information
  - `IsHealthy()`: Provider availability check
  - `GetProvider()`: Provider type identification

- **Supported Providers**:
  - **Ollama** (`ollama.go`): HTTP-based client for local Ollama instances
  - **Azure OpenAI** (`azure.go`): HTTP client for Azure OpenAI services
  - **OpenAI** (`openai.go`): OpenAI API client

- **Configuration** (`config.go`):
  - Loads from environment variables
  - Supports `.env.local` and `.env` files
  - Cascade: .env.local → .env → env vars → defaults
  - Configurable temperature, max_tokens, timeout

- **Provider Chain**:
  - Implements fallback logic across multiple providers
  - Tries each provider until one succeeds
  - Useful for high-availability setups

### 2. AI Chain Layer (`pkg/ai/`)

The AI chain layer provides high-level code analysis operations:

- **Chain Operations**:
  - `QueryNatural()`: Answer questions about code using graph context
  - `AnalyzeEntity()`: Detailed analysis of functions, classes, etc.
  - `GenerateDocs()`: Generate documentation with usage examples
  - `ReviewCode()`: Code review with suggestions
  - `Summarize()`: Summarize file purpose and structure
  - `Chat()`: Multi-turn conversation with context

- **Conversation Context**:
  - Maintains message history
  - Tracks entity and file context
  - Enables multi-turn interactions

### 3. CLI Integration (`main.go`)

Command-line interface for AI features:

```
codecontext ai query <question>      # Natural language questions
codecontext ai analyze <entity>      # Entity analysis
codecontext ai docs <entity>         # Generate docs
codecontext ai review <entity>       # Code review
codecontext ai summarize <file>      # File summary
codecontext ai chat                  # Interactive chat
```

## Configuration

### Environment Variables

```bash
# Provider selection
LLM_PROVIDER=ollama|azure|openai

# Common settings
LLM_MODEL=llama2|gpt-4|etc
LLM_TEMPERATURE=0.7              # 0.0-2.0, higher = more creative
LLM_MAX_TOKENS=2000              # Maximum response length
LLM_TIMEOUT_SECONDS=30           # Request timeout

# Ollama-specific
OLLAMA_BASE_URL=http://localhost:11434

# Azure OpenAI-specific
AZURE_OPENAI_ENDPOINT=https://resource.openai.azure.com/
AZURE_OPENAI_KEY=your-api-key
AZURE_OPENAI_DEPLOYMENT=deployment-name
AZURE_OPENAI_API_VERSION=2024-02-15-preview

# OpenAI-specific
OPENAI_API_KEY=sk-your-api-key
OPENAI_MODEL=gpt-4
```

### Configuration Files

Create `.env.local` (for local overrides) or `.env` (for project defaults):

```sh
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
LLM_MODEL=llama2
LLM_TEMPERATURE=0.7
LLM_MAX_TOKENS=2000
```

**Note**: `.env.local` is in `.gitignore` for credential safety.

## Usage Examples

### 1. Natural Language Queries

Ask questions about your codebase:

```bash
codecontext ai query "what are the main functions in this project?"
codecontext ai query "how does error handling work?"
codecontext ai query "what are the dependencies?"
```

### 2. Entity Analysis

Detailed analysis of specific code elements:

```bash
codecontext ai analyze myFunction
codecontext ai analyze DatabaseConnection
codecontext ai analyze parseJSON
```

### 3. Documentation Generation

Auto-generate documentation:

```bash
codecontext ai docs calculateSum
codecontext ai docs HttpClient
codecontext ai docs validateInput
```

### 4. Code Review

Get AI-powered code review suggestions:

```bash
codecontext ai review fetchUserData
codecontext ai review ConfigManager
codecontext ai review validateEmail
```

### 5. File Summarization

Understand file purpose and structure:

```bash
codecontext ai summarize main.go
codecontext ai summarize database.py
codecontext ai summarize utils/helpers.js
```

### 6. Interactive Chat

Multi-turn conversation about code:

```bash
codecontext ai chat

> What's the purpose of AuthManager?
Assistant: AuthManager is a class that handles...

> Can you show me how to use it?
Assistant: Here's how to use AuthManager...

> What are its dependencies?
Assistant: AuthManager depends on...

> analyze LoginService
Assistant: Here's the detailed analysis...

> exit
```

## How It Works

### Data Flow

```
User Input
    ↓
CLI Handler
    ↓
AI Chain
    ↓
Code Graph Query ←→ LLM Provider
    ↓
Response
```

### Example: Query Flow

1. User runs: `codecontext ai query "what does main do?"`
2. CLI loads LLM configuration
3. AI Chain gets graph statistics for context
4. LLM provider is called with:
   - System prompt (role description)
   - Graph context (file count, entity count, etc.)
   - User question
5. Response is returned to user

### Example: Entity Analysis Flow

1. User runs: `codecontext ai analyze myFunction`
2. CLI loads LLM configuration
3. AI Chain queries code graph:
   - Finds entity "myFunction"
   - Gets call graph (callers, callees)
   - Gets file location and documentation
4. LLM provider is called with:
   - System prompt (role: code analyst)
   - Entity details and relationships
5. Response with analysis is returned

## Provider-Specific Notes

### Ollama

- **Requirements**: Ollama service running locally
- **Setup**:
  ```bash
  # Install from https://ollama.ai
  # Download a model
  ollama pull llama2
  # Start the service
  ollama serve
  ```
- **Advantages**: Free, private, no API keys
- **Performance**: Depends on local hardware

### Azure OpenAI

- **Requirements**: Azure account with OpenAI service
- **Setup**:
  1. Deploy Azure OpenAI resource
  2. Get endpoint and API key
  3. Create deployment
  4. Set environment variables
- **Advantages**: Enterprise security, SOC2/ISO compliance
- **Cost**: Pay-per-use based on tokens

### OpenAI

- **Requirements**: OpenAI API key
- **Setup**:
  1. Sign up at openai.com
  2. Create API key
  3. Set OPENAI_API_KEY environment variable
- **Advantages**: Most capable models (GPT-4), well-documented API
- **Cost**: Pay-per-use based on tokens

## Error Handling

The system includes graceful error handling:

- **Provider Not Available**: Clear error message with setup instructions
- **Configuration Missing**: Lists required environment variables
- **API Errors**: Detailed error messages with retry suggestions
- **Fallback Chain**: Automatically tries next provider if one fails

## Performance Considerations

- **Caching**: Consider caching LLM responses for repeated queries
- **Token Limits**: Configure `LLM_MAX_TOKENS` based on provider limits
- **Timeouts**: Adjust `LLM_TIMEOUT_SECONDS` for slow connections
- **Graph Context**: Larger graphs mean more context for better analysis

## Future Enhancements

Potential improvements:

1. **Caching Layer**: Cache analysis results to reduce API calls
2. **Batch Analysis**: Analyze multiple entities at once
3. **Custom Prompts**: Allow user-defined system prompts
4. **Multi-turn History**: Persistent conversation history
5. **Streaming**: Stream responses for faster feedback
6. **Cost Tracking**: Monitor API usage and costs
7. **RAG Integration**: Integrate with vector databases
8. **Model Fine-tuning**: Fine-tune models on project-specific data

## Troubleshooting

### Provider not responding

```bash
# Check provider health
# For Ollama: curl http://localhost:11434/api/tags

# Check configuration
env | grep LLM_
env | grep OLLAMA_
env | grep AZURE_
env | grep OPENAI_
```

### Unexpected responses

- Check `LLM_TEMPERATURE` (higher = more creative, lower = more deterministic)
- Check `LLM_MAX_TOKENS` (may be too low for complex queries)
- Try with `LLM_PROVIDER=openai` for more capable models

### Configuration not loading

1. Ensure `.env.local` or `.env` exists
2. Check `.env` syntax (no spaces around `=`)
3. Run from project root directory
4. Verify file permissions

## Integration with Code Graph

The AI features leverage the code graph for context:

- **Graph Statistics**: File count, entity count, dependency count
- **Entity Queries**: Find specific functions, classes, etc.
- **Call Graphs**: Understand caller/callee relationships
- **Dependencies**: See how modules interact

This integration enables accurate, context-aware code analysis.


---

## Merged Content from IMPLEMENTATION.md

# Code Graph CLI with MCP Implementation

> Instruction note: this document is implementation reference only. For coding-agent behavior and project instructions, use `.github/AGENTS.md` (canonical) and `.github/CLAUDE.md` (Claude-specific).

## Overview

This implementation adds comprehensive code graph analysis and MCP (Model Context Protocol) server capabilities to the codecontext CLI, enabling Claude and other AI assistants to understand and analyze code structure relationships.

## Architecture

### Project Structure

```
codecontext/
├── main.go                 # CLI entry point with command routing
├── go.mod / go.sum        # Go module dependencies
├── README.md              # Documentation
└── pkg/
    ├── db/                # Database layer (GORM + SQLite)
    │   ├── db.go         # Database operations and queries
    │   └── models.go     # GORM models for ORM
    ├── parser/            # Code parsing for multiple languages
    │   └── parser.go     # Parser implementation
    ├── indexer/           # Graph indexing and queries
    │   └── indexer.go    # Indexer logic
    └── mcp/               # MCP server integration
        └── server.go     # MCP tool definitions and handlers
```

### Core Components

#### 1. **Database Layer** (`pkg/db/`)
- **ORM Framework**: GORM with SQLite driver
- **Auto-Migration**: Schema creation handled automatically by GORM
- **No Manual Setup**: Database is initialized on first use
- **Models**:
  - `File`: Source files with language and hash
  - `Entity`: Code entities (functions, classes, types, variables, interfaces)
  - `Dependency`: File imports/requires with line numbers
  - `EntityRelation`: Relationships between entities (calls, definitions, uses)
- **Indexes**: Strategic indexing on `file_id`, `name`, `type`, `relation_type` for fast queries

#### 2. **Parser** (`pkg/parser/`)
- **Multi-language Support**:
  - **Go**: Functions, types, interfaces, constants, variables
  - **JavaScript/TypeScript**: Functions, classes, interfaces, types, variables
  - **Python**: Functions, classes, imports
- **Extraction**:
  - Entity names, types, signatures, line numbers
  - Import/require statements with paths and line numbers
  - Documentation strings
- **Extensible Design**: Easy to add support for additional languages

#### 3. **Indexer** (`pkg/indexer/`)
- **File Indexing**: Recursively indexes directories
- **Entity Extraction**: Parses and stores code entities
- **Dependency Tracking**: Records file dependencies
- **Query Interface**:
  - `QueryEntity(name)`: Find entities by name
  - `QueryCallGraph(entityID)`: Get call relationships
  - `QueryDependencyGraph(filePath)`: Get file dependencies
  - `GetStats()`: Graph statistics

#### 4. **MCP Server** (`pkg/mcp/`)
- **Tool Definitions**: Standard MCP tools with JSON schemas
- **Available Tools**:
  - `index_directory`: Index a directory to build the code graph
  - `query_entity`: Search for entities by name
  - `query_call_graph`: Get the call graph for an entity
  - `query_dependencies`: Get dependencies for a file
  - `graph_stats`: Get statistics about the indexed code

### CLI Commands

```
codecontext [flags] [command] [args...]

Commands:
  (none)     - Legacy file aggregation (backward compatible)
  index      - Index directory: codecontext index /path
  query      - Query graph: codecontext query [entity|calls|deps] <query>
  stats      - Show statistics: codecontext stats
  mcp        - Start MCP server: codecontext mcp

Flags:
  -ext       - Filter by file extension
  -graph     - Custom database path (default: .codecontext.db)
  -version   - Print version
  -help      - Print help
```

## Key Design Decisions

### 1. **GORM ORM for Zero-Manual-Setup Database**
- **Why**: Users don't need to manually create or manage database schema
- **How**: GORM's `AutoMigrate()` creates tables and indexes automatically
- **Benefit**: Portable, zero-configuration database that works across platforms

### 2. **SQLite as Embedded Database**
- **Why**: No external database server needed, single-file storage
- **Benefit**: Easy distribution, no DevOps overhead, perfect for CLI tools

### 3. **Modular Parser System**
- **Why**: Supports multiple languages without core coupling
- **How**: Language-agnostic `Parse()` function routes to language-specific parsers
- **Extensible**: Adding new languages doesn't modify existing code

### 4. **GORM Models with Foreign Keys**
- **Benefits**:
  - Type-safe queries
  - Automatic relationship loading
  - Cascading deletes
  - Better performance with indexed relationships

### 5. **Backward Compatibility**
- **Design**: Legacy file aggregation works without any database setup
- **Benefit**: Existing users' workflows unchanged

## Database Schema

### Relationships
```
File (1) ---< Entity (many)
File (1) ---< Dependency (many)
Entity (1) ---< EntityRelation (source) ---< Entity (target)
```

### Indexes
- `files(path)` - Unique constraint for file paths
- `entities(file_id)` - Quick lookup of entities in a file
- `entities(name)` - Search by entity name
- `entities(type)` - Filter by entity type
- `dependencies(source_file_id)` - Get imports from a file
- `entity_relations(source_entity_id)` - Get outgoing relationships
- `entity_relations(relation_type)` - Filter by relationship type

## Usage Examples

### Indexing a Project
```bash
codecontext index /path/to/project
# Creates .codecontext.db with parsed entities and relationships
```

### Querying Entities
```bash
# Find all functions named "Handler"
codecontext query entity Handler

# Output:
# Found 2 entities:
#   - ID: 1, Name: Handler, Type: function, File: 3
#   - ID: 5, Name: Handler, Type: function, File: 7
```

### Getting Statistics
```bash
codecontext stats
# Code Graph Statistics:
#   Files:        42
#   Entities:     156
#   Dependencies: 89
#   Relations:    234
```

### MCP Server Integration
```bash
codecontext mcp
# Server ready for Claude to call tools like:
# - index_directory
# - query_entity
# - graph_stats
```

## Data Model Details

### Entity Types
- `function` - Function or method
- `class` - Class definition
- `type` - Type definition (Go types, TypeScript types)
- `interface` - Interface definition
- `variable` - Variable or field declaration
- `constant` - Constant declaration

### Relationship Types
- `calls` - Function A calls function B
- `defines` - Entity defines another
- `uses` - Code uses an entity
- `implements` - Class implements interface
- `extends` - Class extends another

### Dependency Types
- `import` - Go import or Python import
- `require` - CommonJS require
- `from` - Python from...import

## Performance Considerations

1. **Indexes**: Strategic indexes enable fast lookups
2. **GORM Queries**: Efficient SQL generation with prepared statements
3. **Batch Operations**: Indexing uses efficient batch inserts
4. **Lazy Loading**: Relationships loaded on demand

## Future Enhancements

1. **Additional Languages**: Rust, Java, C++, etc.
2. **Cross-file Relationships**: Track actual calls across files
3. **Type Resolution**: Map variables to their types
4. **Usage Analytics**: Find unused code, dead imports
5. **Export Formats**: GraphML, JSON graph exports
6. **Web UI**: Visualization of code graphs

## Maintenance Notes

- **Migration-Free**: GORM handles all schema updates automatically
- **Version Compatible**: Built on stable Go 1.22+ with standard libraries
- **Zero Dependencies** (except GORM/SQLite): No heavy frameworks
- **Testable**: Clear separation of concerns enables unit testing


---

## Merged Content from WORKFLOWS.md

# GitHub Actions Workflows

> Instruction note: this document is CI/workflow reference only. For coding-agent behavior and project instructions, use `.github/AGENTS.md` (canonical) and `.github/CLAUDE.md` (Claude-specific).

This project uses GitHub Actions for automated testing and validation.

## Workflows

### test-cli.yml

**Trigger**: On push to `main` or `claude/**` branches, and on pull requests

**Jobs**:

1. **test-cli**
   - Builds the codecontext binary
   - Tests on the codecontext repository itself
   - Tests on a sample Go project (golang/example)
   - Tests on a mixed-language project (Go + Python + JavaScript)
   - Validates legacy file aggregation mode
   - Verifies version command

2. **test-integration**
   - Runs any available Go unit tests
   - Builds the binary
   - Validates binary size

## Test Coverage

### CLI Functions Tested

- ✅ Build from source
- ✅ Version command
- ✅ File aggregation (legacy mode)
- ✅ Graph indexing on Go projects
- ✅ Graph indexing on external projects
- ✅ Multi-language project indexing
- ✅ Entity queries
- ✅ Graph statistics
- ✅ Database creation

### Languages Tested

- ✅ **Go** (native stdlib AST parser - full support)
  - Functions, methods, types, interfaces, fields
  - Receiver tracking and visibility

- ✅ **Java** (regex-based parser - functions, classes, interfaces)
  - Classes with constructors
  - Methods with parameters
  - Interfaces with method signatures
  - Enums with values
  - JavaDoc extraction

- ✅ **Python** (detected, framework ready)
  - Placeholders for functions and classes

- ✅ **JavaScript/TypeScript** (detected, framework ready)
  - Placeholders for functions and classes

### Test Scenarios

1. **Single Language Projects**
   - codecontext repository (Go)
   - golang/example repository (Go)
   - Java project with classes, interfaces, enums

2. **Mixed Language Projects**
   - Go functions and methods
   - Python classes and functions
   - JavaScript classes and methods

3. **Java Specific Tests**
   - Classes with constructors and methods
   - Interfaces with method signatures
   - Enums with constants
   - JavaDoc comments extraction
   - Method visibility (public, private, protected)

4. **Queries**
   - Entity name search (functions, methods, classes)
   - Graph statistics across multiple languages
   - Language-specific entity detection

## Running Tests Locally

### Run all tests
```bash
go test ./...
```

### Run specific tests
```bash
go test ./pkg/parser/... -v
go test ./pkg/indexer/... -v
```

### Manual CLI testing
```bash
# Build
go build -o codecontext

# Test indexing
./codecontext index .

# Test queries
./codecontext query entity Parse

# Get stats
./codecontext stats
```

## Adding More Tests

To add more test scenarios:

1. **Edit** `.github/workflows/test-cli.yml`
2. **Add a new step** under the appropriate job
3. **Create test data** if needed (temporary directories)
4. **Verify the output** with assertions

Example:
```yaml
- name: Test new feature
  run: |
    cd codecontext
    ./codecontext <command>
    # Verify output or exit code
```

## Success Indicators

A successful run should show:
- ✓ Build successful
- ✓ Index successful
- ✓ Go project indexed successfully
- ✓ Mixed project indexed successfully
- ✓ File aggregation working
- ✓ Version command working

## Troubleshooting

If a workflow fails:

1. Check the **Logs** tab on the GitHub Actions page
2. Look for specific error messages
3. Test locally: `go build && go test ./...`
4. Verify Go version compatibility
5. Check for missing dependencies: `go mod tidy`

## Future Enhancements

Planned additions:
- Code coverage reporting
- Performance benchmarks
- Linting with golangci-lint
- Security scanning
- Release automation
