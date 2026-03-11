# Code Graph CLI with MCP Implementation

## Overview

This implementation adds comprehensive code graph analysis and MCP (Model Context Protocol) server capabilities to the codecontext CLI, enabling Claude and other AI assistants to understand and analyze code structure relationships.

## Architecture

### Project Structure

```
codecontext/
├── main.go                 # CLI entry point with command routing
├── go.mod / go.sum        # Go module dependencies
├── README.md              # Documentation
└── pkg/
    ├── db/                # Database layer (GORM + SQLite)
    │   ├── db.go         # Database operations and queries
    │   └── models.go     # GORM models for ORM
    ├── parser/            # Code parsing for multiple languages
    │   └── parser.go     # Parser implementation
    ├── indexer/           # Graph indexing and queries
    │   └── indexer.go    # Indexer logic
    └── mcp/               # MCP server integration
        └── server.go     # MCP tool definitions and handlers
```

### Core Components

#### 1. **Database Layer** (`pkg/db/`)
- **ORM Framework**: GORM with SQLite driver
- **Auto-Migration**: Schema creation handled automatically by GORM
- **No Manual Setup**: Database is initialized on first use
- **Models**:
  - `File`: Source files with language and hash
  - `Entity`: Code entities (functions, classes, types, variables, interfaces)
  - `Dependency`: File imports/requires with line numbers
  - `EntityRelation`: Relationships between entities (calls, definitions, uses)
- **Indexes**: Strategic indexing on `file_id`, `name`, `type`, `relation_type` for fast queries

#### 2. **Parser** (`pkg/parser/`)
- **Multi-language Support**:
  - **Go**: Functions, types, interfaces, constants, variables
  - **JavaScript/TypeScript**: Functions, classes, interfaces, types, variables
  - **Python**: Functions, classes, imports
- **Extraction**:
  - Entity names, types, signatures, line numbers
  - Import/require statements with paths and line numbers
  - Documentation strings
- **Extensible Design**: Easy to add support for additional languages

#### 3. **Indexer** (`pkg/indexer/`)
- **File Indexing**: Recursively indexes directories
- **Entity Extraction**: Parses and stores code entities
- **Dependency Tracking**: Records file dependencies
- **Query Interface**:
  - `QueryEntity(name)`: Find entities by name
  - `QueryCallGraph(entityID)`: Get call relationships
  - `QueryDependencyGraph(filePath)`: Get file dependencies
  - `GetStats()`: Graph statistics

#### 4. **MCP Server** (`pkg/mcp/`)
- **Tool Definitions**: Standard MCP tools with JSON schemas
- **Available Tools**:
  - `index_directory`: Index a directory to build the code graph
  - `query_entity`: Search for entities by name
  - `query_call_graph`: Get the call graph for an entity
  - `query_dependencies`: Get dependencies for a file
  - `graph_stats`: Get statistics about the indexed code

### CLI Commands

```
codecontext [flags] [command] [args...]

Commands:
  (none)     - Legacy file aggregation (backward compatible)
  index      - Index directory: codecontext index /path
  query      - Query graph: codecontext query [entity|calls|deps] <query>
  stats      - Show statistics: codecontext stats
  mcp        - Start MCP server: codecontext mcp

Flags:
  -ext       - Filter by file extension
  -graph     - Custom database path (default: .codecontext.db)
  -version   - Print version
  -help      - Print help
```

## Key Design Decisions

### 1. **GORM ORM for Zero-Manual-Setup Database**
- **Why**: Users don't need to manually create or manage database schema
- **How**: GORM's `AutoMigrate()` creates tables and indexes automatically
- **Benefit**: Portable, zero-configuration database that works across platforms

### 2. **SQLite as Embedded Database**
- **Why**: No external database server needed, single-file storage
- **Benefit**: Easy distribution, no DevOps overhead, perfect for CLI tools

### 3. **Modular Parser System**
- **Why**: Supports multiple languages without core coupling
- **How**: Language-agnostic `Parse()` function routes to language-specific parsers
- **Extensible**: Adding new languages doesn't modify existing code

### 4. **GORM Models with Foreign Keys**
- **Benefits**:
  - Type-safe queries
  - Automatic relationship loading
  - Cascading deletes
  - Better performance with indexed relationships

### 5. **Backward Compatibility**
- **Design**: Legacy file aggregation works without any database setup
- **Benefit**: Existing users' workflows unchanged

## Database Schema

### Relationships
```
File (1) ---< Entity (many)
File (1) ---< Dependency (many)
Entity (1) ---< EntityRelation (source) ---< Entity (target)
```

### Indexes
- `files(path)` - Unique constraint for file paths
- `entities(file_id)` - Quick lookup of entities in a file
- `entities(name)` - Search by entity name
- `entities(type)` - Filter by entity type
- `dependencies(source_file_id)` - Get imports from a file
- `entity_relations(source_entity_id)` - Get outgoing relationships
- `entity_relations(relation_type)` - Filter by relationship type

## Usage Examples

### Indexing a Project
```bash
codecontext index /path/to/project
# Creates .codecontext.db with parsed entities and relationships
```

### Querying Entities
```bash
# Find all functions named "Handler"
codecontext query entity Handler

# Output:
# Found 2 entities:
#   - ID: 1, Name: Handler, Type: function, File: 3
#   - ID: 5, Name: Handler, Type: function, File: 7
```

### Getting Statistics
```bash
codecontext stats
# Code Graph Statistics:
#   Files:        42
#   Entities:     156
#   Dependencies: 89
#   Relations:    234
```

### MCP Server Integration
```bash
codecontext mcp
# Server ready for Claude to call tools like:
# - index_directory
# - query_entity
# - graph_stats
```

## Data Model Details

### Entity Types
- `function` - Function or method
- `class` - Class definition
- `type` - Type definition (Go types, TypeScript types)
- `interface` - Interface definition
- `variable` - Variable or field declaration
- `constant` - Constant declaration

### Relationship Types
- `calls` - Function A calls function B
- `defines` - Entity defines another
- `uses` - Code uses an entity
- `implements` - Class implements interface
- `extends` - Class extends another

### Dependency Types
- `import` - Go import or Python import
- `require` - CommonJS require
- `from` - Python from...import

## Performance Considerations

1. **Indexes**: Strategic indexes enable fast lookups
2. **GORM Queries**: Efficient SQL generation with prepared statements
3. **Batch Operations**: Indexing uses efficient batch inserts
4. **Lazy Loading**: Relationships loaded on demand

## Future Enhancements

1. **Additional Languages**: Rust, Java, C++, etc.
2. **Cross-file Relationships**: Track actual calls across files
3. **Type Resolution**: Map variables to their types
4. **Usage Analytics**: Find unused code, dead imports
5. **Export Formats**: GraphML, JSON graph exports
6. **Web UI**: Visualization of code graphs

## Maintenance Notes

- **Migration-Free**: GORM handles all schema updates automatically
- **Version Compatible**: Built on stable Go 1.22+ with standard libraries
- **Zero Dependencies** (except GORM/SQLite): No heavy frameworks
- **Testable**: Clear separation of concerns enables unit testing
