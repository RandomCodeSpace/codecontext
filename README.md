# codecontext

Aggregate source files into a single context block — useful for feeding code to AI assistants.

## Installation

Requires [Go 1.22+](https://go.dev/dl/).

**Latest stable release:**
```sh
go install github.com/RandomCodeSpace/codecontext@latest
```

**Specific version:**
```sh
go install github.com/RandomCodeSpace/codecontext@v1.2.3
```

**Pre-release:**
```sh
go install github.com/RandomCodeSpace/codecontext@v1.2.3-beta.1
```

The binary is placed in `$(go env GOPATH)/bin`. Make sure that directory is on your `PATH`:
```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Usage

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

## Output

Each file is printed with a header followed by its contents:

```
=== main.go ===
package main
...

=== go.mod ===
module github.com/RandomCodeSpace/codecontext
...
```

Hidden directories (e.g. `.git`) are skipped automatically.
