# GitHub Copilot Instructions — codecontext

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
See `CLAUDE.md` in the project root for comprehensive task guides (adding parsers, MCP tools, DB schema changes, web UI modifications).
