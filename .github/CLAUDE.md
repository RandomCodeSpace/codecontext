# CLAUDE.md — AI Agent Instructions (Python)

## Quick Reference
```bash
uv sync --all-extras
uv run pytest -q
uv run python -m codecontext -version
uv run python -m codecontext index .
uv run python -m codecontext stats
uv run python -m codecontext query entity Parse
uv run python -m codecontext web 8080        # Web UI + HTTP MCP (/mcp)
uv run python -m codecontext mcp             # MCP stdio (CLI integration)
```

## Architecture
- `src/codecontext/cli.py` command routing
- `src/codecontext/db.py` SQLite-backed graph model and queries
- `src/codecontext/db_cogdb.py` CogDB graph database backend (cross-platform)
- `src/codecontext/parser.py` multi-language parser adapters
- `src/codecontext/indexer.py` hash-based indexing pipeline
- `src/codecontext/mcp.py` MCP tool API and stdio server
- `src/codecontext/llm.py` provider abstraction (`ollama`, `openai`, `azure`, `mock`)
- `src/codecontext/ai.py` NL query/analyze/docs/review/summarize/chat chains
- `src/codecontext/web.py` web endpoints (`/api/*`) plus MCP HTTP (`/mcp`)

## Testing
- Default: `uv run pytest -q`
- Focused examples:
  - `uv run pytest -q tests/test_mcp_tools.py`
  - `uv run pytest -q tests/test_ai_commands.py`
  - `uv run pytest -q tests/test_web_api.py`

## Conventions
- Python 3.12+
- Keep interfaces simple and explicit
- Preserve command/tool contract compatibility
- Prefer adding tests for every user-visible behavior change
