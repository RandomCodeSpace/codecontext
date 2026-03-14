# codecontext Implementation (Python)

## Overview
codecontext is implemented as a Python CLI + library that indexes source code into SQLite and exposes graph operations via CLI, MCP, AI, and web routes.

## Component Map
- `src/codecontext/db.py`: schema creation, CRUD, graph/stat query helpers
- `src/codecontext/parser.py`: language detection and parser adapters
- `src/codecontext/indexer.py`: directory walk, file hashing, parse -> DB write flow
- `src/codecontext/mcp.py`: MCP tool handlers + stdio server runtime
- `src/codecontext/llm.py`: provider config/client abstraction
- `src/codecontext/ai.py`: AI workflows built on provider + indexer context
- `src/codecontext/web.py`: FastAPI web routes, JSON graph endpoints, and HTTP MCP endpoint (`/mcp`)

## Data Model
- `files`
- `entities`
- `dependencies`
- `entity_relations`

## Key Behaviors
1. Hash-based skip for unchanged files.
2. Dependency extraction per file.
3. Entity relation storage (`defines`, `calls` etc. as available).
4. Graph query APIs for CLI and MCP tools.

## Build/Test
```bash
uv sync --all-extras
uv run pytest -q
```
