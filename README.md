# codecontext

Aggregate source files into a single context block — useful for feeding code to AI assistants. Now with **code graph analysis** and **MCP server** support.

## Features

- **File Aggregation**: Combine source files into a single context block (original functionality)
- **Code Graph**: Index and analyze code relationships with an embedded SQLite database
- **MCP Server**: Expose code graph queries via Model Context Protocol for Claude integration
- **Multi-language Support**: Parse Go, JavaScript/TypeScript, and Python code
- **Automatic Schema**: GORM-based ORM eliminates manual database setup
- **Entity Extraction**: Automatically detect functions, classes, types, variables, and their relationships

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

Build and query a code graph:

```sh
# Index a directory
codecontext index /path/to/project

# Query for an entity (function, class, etc.)
codecontext query entity functionName

# Query dependencies for a file
codecontext query deps main.go

# Get call graph for an entity (by ID)
codecontext query calls 1

# Show graph statistics
codecontext stats
```

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
