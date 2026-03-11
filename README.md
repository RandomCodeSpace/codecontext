# codecontext

Aggregate source files into a single context block — useful for feeding code to AI assistants. Now with **code graph analysis** and **MCP server** support.

## Features

- **File Aggregation**: Combine source files into a single context block (original functionality)
- **Code Graph**: Index and analyze code relationships with an embedded SQLite database
- **MCP Server**: Expose code graph queries via Model Context Protocol for Claude integration
- **Multi-language Auto-Detection**: Automatically detect and parse **Go, Python, Java, JavaScript, and TypeScript** code in mixed-language projects
- **Recursive Indexing**: Scan entire project directories, automatically detecting language and extracting code structure
- **Automatic Schema**: GORM-based ORM eliminates manual database setup
- **Entity Extraction**: Automatically detect functions, classes, types, variables, interfaces, methods, and their relationships

## Installation

Requires [Go 1.22+](https://go.dev/dl/).

**Latest stable release:**
```sh
go install github.com/RandomCodeSpace/codecontext@latest
```

**Build from source:**
```sh
git clone https://github.com/RandomCodeSpace/codecontext.git
cd codecontext
go build -o codecontext
```

## Usage

### File Aggregation (Legacy)

Combine source files into a single context block:

```sh
# All files in the current directory
codecontext .

# Filter by file extension
codecontext -ext .go,.md .

# Specific files
codecontext main.go go.mod README.md

# Print version
codecontext -version
```

### Code Graph Analysis

Build and query a code graph with automatic language detection:

```sh
# Index a directory (auto-detects all languages)
codecontext index /path/to/project

# Index a multi-language monorepo
codecontext index /path/to/monorepo
# Automatically finds: Go files, Python files, Java classes, JS/TS modules

# Query for an entity (function, class, etc.)
codecontext query entity functionName

# Query dependencies for a file
codecontext query deps main.go

# Get call graph for an entity (by ID)
codecontext query calls 1

# Show graph statistics
codecontext stats
```

### Multi-Language Projects

The `index` command automatically detects and processes files in multiple languages:

```sh
# Index a mixed-language project
codecontext index my-project/

# This will find and parse:
# - Go files (.go)
# - Python files (.py)
# - Java files (.java)
# - JavaScript/TypeScript files (.js, .ts, .jsx, .tsx)

# Result:
# ✓ Indexing complete. Database: .codecontext.db
#
# Code Graph Statistics:
#   Files:        245
#   Entities:     3421
#   Dependencies: 1204
#   Relations:    0
```

**Supported Languages**:
- **Go** - Full AST parsing with stdlib, functions, methods, types, interfaces, fields
- **Python** - Function and class detection, decorator tracking
- **Java** - Class, interface, enum, record, and method extraction
- **JavaScript/TypeScript** - Functions, classes, interfaces (TS), type aliases, arrow functions, async functions
- **More languages coming** - Framework supports easy addition of new parsers

### MCP Server

Start an MCP server for Claude integration:

```sh
codecontext mcp
```

This exposes the following MCP tools:
- `index_directory`: Index a directory to build the code graph
- `query_entity`: Search for entities by name
- `query_call_graph`: Get the call graph for an entity
- `query_dependencies`: Get dependencies for a file
- `graph_stats`: Get statistics about the indexed code

## Database

By default, the code graph is stored in `.codecontext.db` (SQLite). Use the `-graph` flag to specify a different location:

```sh
codecontext -graph /custom/path/graph.db index .
codecontext -graph /custom/path/graph.db stats
```

## Output Examples

### File Aggregation

Each file is printed with a header followed by its contents:

```
=== main.go ===
package main
...

=== go.mod ===
module github.com/RandomCodeSpace/codecontext
...
```

### Query Results

```
$ codecontext query entity myFunction
Found 2 entities:
  - ID: 1, Name: myFunction, Type: function, File: 3
  - ID: 5, Name: myFunction, Type: function, File: 7

$ codecontext stats
Code Graph Statistics:
  Files:        42
  Entities:     156
  Dependencies: 89
  Relations:    234
```

## Configuration

Both legacy (file aggregation) and new (graph) features use the same database and can work in parallel. The `-ext` flag filters files during both aggregation and indexing.

Hidden directories (e.g. `.git`) are skipped automatically.
