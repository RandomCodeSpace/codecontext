# codecontext (Python)

`codecontext` indexes source repositories into a SQLite-backed code graph and exposes:
- CLI workflows (`index`, `query`, `stats`, `docs`, `ai`, `web`, `mcp`)
- MCP-compatible tool server (stdio via `mcp`, HTTP via `web`)
- Web API/UI for graph/stats/tree exploration

This repository is now Python-first and uses `uv` for environment and command execution.

Dependency management is defined in `pyproject.toml` and locked in `uv.lock`.

## Requirements

- Python 3.12+
- [uv](https://docs.astral.sh/uv/)

## Quick Start

```bash
uv sync --all-extras
uv run python -m codecontext -version
```

## Common Commands

```bash
uv run python -m codecontext index .
uv run python -m codecontext stats
uv run python -m codecontext query entity Parse

uv run python -m codecontext docs -output docs.md
LLM_PROVIDER=mock uv run python -m codecontext docs -ai -output ai-docs.md

LLM_PROVIDER=mock uv run python -m codecontext ai query "what does this repo do"
uv run python -m codecontext web 8080  # Web UI + API + HTTP MCP at /mcp
uv run python -m codecontext mcp       # MCP stdio for CLI/editor integrations
```

When running `web`, all HTTP surfaces share one port:
- UI: `GET /`
- Web API: `GET /api/*`
- MCP HTTP: `POST /mcp`

## LLM Providers

Supported providers:
- `ollama` (default)
- `openai`
- `azure`
- `mock` (useful for local tests)

Environment variables:

```bash
# Provider selection
export LLM_PROVIDER=ollama
export LLM_MODEL=llama2

# Common settings
export LLM_TEMPERATURE=0.7
export LLM_MAX_TOKENS=2000
export LLM_TIMEOUT_SECONDS=30

# Ollama
export OLLAMA_BASE_URL=http://localhost:11434

# OpenAI
export OPENAI_API_KEY=...
export OPENAI_MODEL=gpt-4o-mini

# Azure OpenAI
export AZURE_OPENAI_ENDPOINT=...
export AZURE_OPENAI_KEY=...
export AZURE_OPENAI_DEPLOYMENT=...
export AZURE_OPENAI_API_VERSION=2024-02-15-preview
```

## Development

```bash
uv run pytest -q
uv run pytest -q tests/test_web_api.py tests/test_mcp_tools.py
```

## Cross-Platform Development (Linux + Windows)

Use this quick checklist to ensure the project is installable and usable on both platforms:

1. Sync and run tests:

```bash
uv sync --all-extras
uv run pytest -q
```

2. Build distributables and verify local install:

```bash
uv build
python -m pip install dist/*.whl
codecontext -version
```

3. Validate core command paths:

```bash
codecontext index .
codecontext stats
codecontext web 8080
```

Backend note:
- Non-Windows default backend: `falkordblite`
- Windows default backend: `sqlite` (the `falkordblite` dependency is not available on win32)

CI enforces this on both Linux and Windows via `.github/workflows/test-cli.yml`.

## Publish Public Package (PyPI)

This repository publishes Python packages through GitHub Actions with:
- Primary path: `.github/workflows/release.yml` (builds, creates release, uploads artifacts, and publishes)
	- all versions (stable and prerelease) publish to PyPI
- Fallback path: `.github/workflows/publish-pypi.yml` (auto on release event or manual run by tag)

### One-time setup

1. Create project on PyPI:
	- Create an account on PyPI.
	- Create a project named `ossr-codecontext`.

2. Configure Trusted Publishing on PyPI:
	- Publisher type: GitHub
	- Repository: your org/user + repo name
	- Workflow: `release.yml`
	- Environment: leave empty unless you use one

3. Ensure the package metadata in `pyproject.toml` is public-ready:
	- `name`, `version`, `description`, `readme`, `requires-python`, `authors`
	- add `license`, `classifiers`, `urls` if needed

Package naming note:
- PyPI distribution name: `ossr-codecontext`
- CLI command: `codecontext`
- Python import package: `codecontext`

### Release flow

1. Run the Release workflow manually with a version like `v1.2.3` or `v1.2.3-beta.1`.
2. The workflow builds and attaches artifacts to the GitHub Release.
3. Publishing occurs in the same release workflow run:
	- stable and prerelease -> PyPI

If release-event automation is blocked by repository token policy, run `publish-pypi.yml` manually and pass the release tag.

If a release job is re-run for the same version, existing files on PyPI are skipped. In normal use, publish a new version for each release.

### Verify install

Install from PyPI:

```bash
pip install ossr-codecontext
```

## Notes

- Default graph database: `.codecontext.db`
- Override with `-graph /path/to/graph.db`
- Startup helper script: `./codecontext_startup.sh`
