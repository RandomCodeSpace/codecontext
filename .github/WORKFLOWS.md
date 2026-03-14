# GitHub Actions Workflows

This project uses GitHub Actions for automated testing and validation.

## Workflows

### test-cli.yml

**Trigger**: On push to `main` or `claude/**` branches, and on pull requests

**Jobs**:

<<<<<<< Updated upstream
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
=======
What it does:
1. Validates version format.
2. Syncs dependencies and runs the full test suite.
3. Sets package version from the provided tag.
4. Builds distribution artifacts.
5. Creates and pushes tag.
6. Creates GitHub release notes and uploads artifacts.
7. Publishes prereleases to TestPyPI and stable releases to PyPI in the same workflow run.

### `publish-pypi.yml`
Trigger: GitHub Release published event, or manual `workflow_dispatch` with a tag.
>>>>>>> Stashed changes

## Test Coverage

<<<<<<< Updated upstream
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
=======
Note:
- `release.yml` is the primary publish path.
- `publish-pypi.yml` is a fallback/manual republish path for an existing release tag.

## Local Equivalent Commands
>>>>>>> Stashed changes
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
