# GitHub Actions Workflows (Python)

## Active Workflows

### `test-cli.yml`
Trigger: push/PR on `main` and `claude/**`.

Runs on:
- Linux (`ubuntu-latest`)
- Windows (`windows-latest`)

What it does:
1. Installs `uv` and Python.
2. Runs `uv sync --all-extras`.
3. Executes CLI smoke checks.
4. Runs targeted pytest suites (including web API tests that cover `/mcp` on the web app).
5. Builds wheel artifacts, installs from wheel, and validates `codecontext -version`.
6. Validates indexing and queries on repository and mixed-language sample project using CLI stats/query outputs (backend-agnostic; does not rely on `.codecontext.db` file presence).

### `release.yml`
Trigger: manual dispatch with semantic version input.

What it does:
1. Validates version format.
2. Syncs dependencies and runs the full test suite.
3. Sets package version from the provided tag.
4. Builds distribution artifacts.
5. Creates and pushes tag.
6. Creates GitHub release notes and uploads artifacts.
7. Publishes all release versions (stable and prerelease) to PyPI in the same workflow run.

### `publish-pypi.yml`
Trigger: GitHub Release published event, or manual `workflow_dispatch` with a tag.

What it does:
1. Downloads the wheel and sdist artifacts from the published release.
2. Publishes the selected release artifacts to PyPI.
3. Supports both stable and prerelease tags.
4. Uses trusted publishing via OIDC (no API token required once PyPI is configured).

Note:
- Primary path is `release.yml`.
- `publish-pypi.yml` is a fallback/manual republish path for an existing tag.

## Local Equivalent Commands
```bash
uv sync --all-extras
uv run pytest -q
uv run python -m codecontext -version
uv run python -m codecontext index .
uv run python -m codecontext stats
```

PyPI distribution name:
- `ossr-codecontext`

Runtime names remain:
- CLI command: `codecontext`
- Python package import: `codecontext`
