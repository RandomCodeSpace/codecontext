# GitHub Copilot Instructions — codecontext (Python)

## Project Overview
codecontext is a Python CLI that indexes source code into a SQLite-backed graph and exposes it via MCP, a web API/UI, and AI analysis commands.

## Build & Test Commands
```bash
uv sync --all-extras
uv run pytest -q
uv run python -m codecontext -version
uv run python -m codecontext index .
uv run python -m codecontext stats
```

## Code Conventions
- Python 3.12+
- Prefer simple modules and explicit data models (`dataclasses`)
- Keep error messages actionable and user-facing CLI output concise
- Maintain CLI compatibility for existing flags/commands
- Keep it simple, avoid unnecessary abstractions

## Architecture Summary
| Path | Role |
|---|---|
| `src/codecontext/cli.py` | CLI entry point and command routing |
| `src/codecontext/db.py` | SQLite schema + graph queries |
| `src/codecontext/parser.py` | Multi-language parsing + dependency extraction |
| `src/codecontext/indexer.py` | File indexing, hashing, graph population |
| `src/codecontext/mcp.py` | MCP-compatible tools + stdio server |
| `src/codecontext/llm.py` | LLM provider abstraction |
| `src/codecontext/ai.py` | AI chains and chat workflows |
| `src/codecontext/web.py` | FastAPI web routes (`/api/*`) + MCP HTTP (`/mcp`) |

## Key Dependencies
- `fastapi`, `uvicorn`
- `httpx`
- `openai`
- `tree-sitter`
- `pytest`

## Testing Requirements
- Run `uv run pytest -q` after code changes.
- For command-specific edits, run focused tests in `tests/`.
- CI: `.github/workflows/test-cli.yml` validates CLI on sample projects.
