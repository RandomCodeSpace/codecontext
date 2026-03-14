# AGENTS.md — Universal AI Agent Instructions (Python)

## What This Project Is
codecontext is a Python CLI that builds a SQLite code graph from source files and exposes query tooling through CLI, MCP, AI, and web APIs.

## Essential Commands
```bash
uv sync --all-extras
uv run pytest -q
uv run python -m codecontext -version
uv run python -m codecontext index .
uv run python -m codecontext stats
uv run python -m codecontext web 8080        # Web UI + HTTP MCP (/mcp)
uv run python -m codecontext mcp             # MCP stdio (CLI integration)
```

## Project Structure
- `src/codecontext/cli.py` — command routing
- `src/codecontext/db.py` — SQLite models/queries
- `src/codecontext/parser.py` — source parsing and dependency extraction
- `src/codecontext/indexer.py` — indexing pipeline
- `src/codecontext/mcp.py` — MCP tool handlers and stdio server
- `src/codecontext/llm.py` — provider config and clients
- `src/codecontext/ai.py` — AI analysis chains
- `src/codecontext/web.py` — FastAPI endpoints
- `tests/` — CLI, indexer, docs, MCP, AI, web tests

## Coding Rules
1. Keep behavior-compatible CLI flags and command semantics.
2. Prefer deterministic unit tests and clear fixtures.
3. Preserve SQLite schema and graph-query semantics.
4. Keep MCP tool contracts stable.
5. Prefer `uv run ...` for all developer commands.

## Operational Notes
- Default graph DB is `.codecontext.db`; override via `-graph`.
- Startup helper script: `./codecontext_startup.sh` (Python runtime).
- `LLM_PROVIDER=mock` is recommended for local deterministic tests.
