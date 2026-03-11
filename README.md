# codecontext

Aggregate source files into a single context block — useful for feeding code to AI assistants. Now with **code graph analysis**, **AI-powered code analysis**, and **MCP server** support.

## Features

- **File Aggregation**: Combine source files into a single context block (original functionality)
- **Code Graph**: Index and analyze code relationships with an embedded SQLite database
- **AI-Powered Analysis**: Analyze code using LLMs with multi-provider support (Ollama, Azure OpenAI, OpenAI)
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

### AI-Powered Code Analysis

Analyze your code using LLMs with automatic integration with your code graph:

```sh
# Natural language queries about code
codecontext ai query "what does the main function do?"

# Detailed analysis of a specific entity
codecontext ai analyze myFunction

# Generate documentation
codecontext ai docs MyClass

# Get code review suggestions
codecontext ai review calculateSum

# Summarize a file's purpose
codecontext ai summarize main.go

# Interactive chat mode
codecontext ai chat
```

**AI Configuration**

The AI features support multiple LLM providers. Configure via environment variables or `.env` files:

```sh
# Use Ollama (default, runs locally)
export LLM_PROVIDER=ollama
export OLLAMA_BASE_URL=http://localhost:11434
export LLM_MODEL=llama2

# Use Azure OpenAI
export LLM_PROVIDER=azure
export AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com/
export AZURE_OPENAI_KEY=your-api-key
export AZURE_OPENAI_DEPLOYMENT=deployment-name

# Use OpenAI
export LLM_PROVIDER=openai
export OPENAI_API_KEY=sk-your-api-key
export OPENAI_MODEL=gpt-4

# Common settings
export LLM_TEMPERATURE=0.7
export LLM_MAX_TOKENS=2000
export LLM_TIMEOUT_SECONDS=30
```

**Configuration Priority** (first available wins):
1. `.env.local` (for local overrides, add to .gitignore)
2. `.env` (in project root)
3. Environment variables
4. Built-in defaults

**Supported Providers**:
- **Ollama**: Free, runs locally. Download models: `ollama pull llama2`
- **Azure OpenAI**: Enterprise option with data privacy
- **OpenAI**: GPT-3.5/GPT-4 models via API

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
- `get_docs`: Get human-readable documentation for a project, file, or entity

### Sample Use Case — Generate a Business Document with GitHub Copilot

Once the MCP server is running and your codebase is indexed, paste this prompt
into GitHub Copilot Chat (or any MCP-capable agent) to produce a full
**Technical Business Overview** document:

```
You have access to a codecontext MCP server. Use its tools to gather
information about this codebase and produce a structured business document.

Step 1 — Gather data

Run these MCP tools in order:

1. graph_stats — get overall project stats (language breakdown, entity counts)
2. get_docs with scope=project — get a high-level project overview
3. get_docs with scope=file, format=Markdown — repeat for the 3–5 most
   important files (entry points, core modules, public API surface)
4. query_dependencies for each of those key files — understand coupling
5. For any public-facing function you find, run query_call_graph to trace
   how it works end-to-end

Step 2 — Generate the document

Using all data collected above, write a Technical Business Overview with:

1. Executive Summary
   - What the system does (2–3 sentences, non-technical)
   - Primary value proposition

2. System Architecture
   - Languages and frameworks used (from graph_stats)
   - Component breakdown (from project-level docs + file docs)
   - A dependency map in Mermaid diagram format

3. Public API / Capabilities
   - Each major public function or module with: purpose, inputs, outputs
   - Format as a table

4. Integration Guide
   - How to connect to / use this system
   - Key entry points identified from call graphs

5. Technical Debt & Risk
   - Deeply coupled files (high dependency counts)
   - Complexity hotspots (entities with large call graphs)

6. Appendix — Entity Index
   - Table of name | type | file | description

Format the final document as Markdown.
```

Swap out the Step 2 template for other document types — API reference,
onboarding guide, architecture decision record — while keeping Step 1
unchanged.

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
