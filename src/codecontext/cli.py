import argparse
import json
import os
import sys
from pathlib import Path

from .ai import Chain, ConversationContext, ConversationMessage
from . import __version__
from .backends import normalize_backend_name, open_backend
from .indexer import Indexer
from .llm import load_config, new_provider
from .mcp import run_http, run_stdio
from .storage import StorageBackend
from .web import create_app


def _configure_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if callable(reconfigure):
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


def _default_backend() -> str:
    return "sqlite"

USAGE = """codecontext - aggregate source files and build code graphs with AI analysis

Usage:
  codecontext [flags] [command] [args...]
    codecontext index <path> [-jobs N]

Commands:
  (no command)         Aggregate source files into a single context block
  index                Index a directory to build code graph
  query                Query the code graph
  ai                   Analyze code with AI
  docs                 Generate documentation for the whole project
  stats                Show graph statistics
    web                  Start web UI + MCP HTTP endpoint on one port
    mcp                  Start MCP stdio server (for Claude integration)

Flags:
    -ext string          Comma-separated file extensions to include (e.g. .go,.ts)
    -graph string        Path to graph database (default: .codecontext.db)
    -backend string      Storage backend: sqlite or cogdb (default: sqlite)
    -verbose             Enable verbose logging
    -version             Print version and exit
    -help                Print this help message

Docs Flags (codecontext docs):
  -ai                  Use AI to write entity descriptions
  -prompt string       Custom instruction for AI output style (only with -ai)
                       e.g. "Use JSDoc format" or "One sentence per entity"
  -output string       Write documentation to this file (default: stdout)

AI Subcommands:
  codecontext ai query <query>           Ask a natural language question about the code
  codecontext ai analyze <entity>        Detailed analysis of an entity
  codecontext ai docs <entity>           Generate documentation for an entity
  codecontext ai review <entity>         Code review suggestions for an entity
  codecontext ai summarize <file>        Summary of a file's purpose
  codecontext ai chat                    Multi-turn conversation about code
"""


def _parse_ext_filter(ext_filter: str) -> set[str] | None:
    if not ext_filter:
        return None
    exts: set[str] = set()
    for value in ext_filter.split(","):
        ext = value.strip()
        if not ext:
            continue
        if not ext.startswith("."):
            ext = f".{ext}"
        exts.add(ext)
    return exts


def _walk_dir(directory: Path, exts: set[str] | None) -> list[Path]:
    files: list[Path] = []
    for root, dirs, names in os.walk(directory):
        dirs[:] = [d for d in dirs if not d.startswith(".")]
        for name in names:
            path = Path(root) / name
            if exts and path.suffix not in exts:
                continue
            files.append(path)
    return files


def _print_file(path: Path) -> None:
    data = path.read_text(encoding="utf-8", errors="replace")
    print(f"=== {path} ===")
    print(data)


def legacy_aggregate(path_arg: str, ext_filter: str) -> int:
    exts = _parse_ext_filter(ext_filter)
    path = Path(path_arg)
    if not path.exists():
        print(f"error: {path_arg} does not exist", file=sys.stderr)
        return 1

    files: list[Path]
    if path.is_dir():
        files = _walk_dir(path, exts)
    else:
        files = [path]

    if not files:
        print("no files found", file=sys.stderr)
        return 1

    for file_path in files:
        try:
            _print_file(file_path)
        except OSError as err:
            print(f"error reading {file_path}: {err}", file=sys.stderr)
    return 0


def _format_count(value: int) -> str:
    def compact(n: float, suffix: str) -> str:
        text = f"{n:.1f}".rstrip("0").rstrip(".")
        return f"{value} ({text}{suffix})"

    if value >= 1_000_000_000:
        return compact(value / 1_000_000_000.0, "B")
    if value >= 1_000_000:
        return compact(value / 1_000_000.0, "M")
    if value >= 1_000:
        return compact(value / 1_000.0, "K")
    return str(value)


def _print_stats_summary(stats: dict[str, int], graph_db: str | None = None) -> None:
    print("📈 Code Graph Statistics")
    print(f"  📁 Files:        {stats['files']}")
    print(f"  📏 LOC:          {_format_count(stats['lines_of_code'])}")
    print(f"  🧮 Tokens:       {_format_count(stats['tokens'])}")
    print(f"  🧩 Entities:     {stats['entities']}")
    print(f"  🔗 Relations:    {stats['relations']}")
    print(f"  📦 Dependencies: {stats['dependencies']}")
    if graph_db is not None:
        print(f"  💾 Database:     {graph_db}")


def _open_indexer(graph_db: str, backend: str, verbose: bool) -> tuple[StorageBackend, Indexer]:
    normalized_backend = normalize_backend_name(backend)
    database = open_backend(normalized_backend, graph_db, verbose)
    indexer = Indexer(database, normalized_backend)
    indexer.set_verbose(verbose)
    return database, indexer


def _standard_docs(indexer: Indexer) -> str:
    files = indexer.db.get_files()
    entities = indexer.db.get_all_entities()
    stats = indexer.get_stats()

    entities_by_file: dict[int, list[object]] = {}
    for entity in entities:
        entities_by_file.setdefault(entity.file_id, []).append(entity)

    lines: list[str] = []
    lines.append("# Project Documentation")
    lines.append("")
    lines.append(
        f"_Generated from indexed database - {stats['files']} files, {stats['entities']} entities_"
    )
    lines.append("")
    lines.append("---")
    lines.append("")

    for file in files:
        file_entities = entities_by_file.get(file.id, [])
        if not file_entities:
            continue

        lines.append(f"## `{file.path}` - {file.language}")
        lines.append("")

        for entity in file_entities:
            heading = f"`{entity.name}`"
            if entity.parent:
                heading = f"`{entity.parent}.{entity.name}`"

            lines.append(
                f"### {heading} - {entity.type} - lines {entity.start_line}-{entity.end_line}"
            )
            lines.append("")

            if entity.signature:
                lines.append("```")
                lines.append(entity.signature)
                lines.append("```")
                lines.append("")

            if entity.visibility:
                lines.append(f"- **Visibility:** {entity.visibility}")
            if entity.kind and entity.kind != entity.type:
                lines.append(f"- **Kind:** {entity.kind}")
            if entity.documentation:
                lines.append("")
                lines.append(entity.documentation)
            lines.append("")

        lines.append("---")
        lines.append("")

    return "\n".join(lines)


def handle_index(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    parser = argparse.ArgumentParser(prog="codecontext index", add_help=False)
    parser.add_argument("path", nargs="?")
    parser.add_argument("-parallel", action="store_true")  # kept for backward compatibility; parallel is now default
    parser.add_argument("-jobs", type=int, default=0)
    parsed, remaining = parser.parse_known_args(args)
    if remaining:
        print(f"unexpected index args: {' '.join(remaining)}", file=sys.stderr)
        return 1
    if not parsed.path:
        print("usage: codecontext index <path> [-parallel] [-jobs N]", file=sys.stderr)
        return 1
    if parsed.jobs < 0:
        print("-jobs must be >= 0", file=sys.stderr)
        return 1

    # Default to parallel parse workers; use -jobs 1 to force sequential behavior.
    cpu = os.cpu_count() or 1
    workers = parsed.jobs if parsed.jobs > 0 else max(1, min(cpu, 8))

    dir_path = parsed.path
    print(f"Opening database: {graph_db}")
    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    try:
        print(f"Indexing directory: {dir_path}")
        indexer.set_parse_workers(workers)
        mode = "parallel" if workers > 1 else "sequential"
        print(f"Index mode: {mode} (workers={workers})")
        indexer.index_directory(dir_path)
        stats = indexer.get_stats()
    except KeyboardInterrupt:
        print("Indexing canceled by user", file=sys.stderr)
        return 130
    except Exception as err:  # noqa: BLE001
        print(f"error indexing directory: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()

    print("✅ Indexing complete")
    _print_stats_summary(stats, graph_db)
    return 0


def handle_query(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    if len(args) < 2:
        print("usage: codecontext query <type> <query>", file=sys.stderr)
        print("types: entity, calls, deps", file=sys.stderr)
        return 1

    query_type = args[0]
    query = args[1]

    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    try:
        if query_type == "entity":
            entities = indexer.query_entity(query)
            print(f"Found {len(entities)} entities")
            for entity in entities:
                print(
                    f"  - [{entity.type}] {entity.name} "
                    f"(ID={entity.id}, file={entity.file_id}, lines {entity.start_line}-{entity.end_line})"
                )
                if entity.signature:
                    print(f"    Signature: {entity.signature}")
                if entity.parent:
                    print(f"    Parent:    {entity.parent}")
            return 0

        if query_type == "calls":
            try:
                entity_id = int(query)
            except ValueError:
                print(f"invalid entity ID: {query}", file=sys.stderr)
                return 1
            graph = indexer.query_call_graph(entity_id)
            print(json.dumps(graph, indent=2))
            return 0

        if query_type == "deps":
            graph = indexer.query_dependency_graph(query)
            print(json.dumps(graph, indent=2))
            return 0

        print(f"unknown query type: {query_type}", file=sys.stderr)
        return 1
    except Exception as err:  # noqa: BLE001
        print(f"query failed: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()


def handle_docs(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    parser = argparse.ArgumentParser(prog="codecontext docs", add_help=False)
    parser.add_argument("-ai", action="store_true", dest="use_ai")
    parser.add_argument("-prompt", default="")
    parser.add_argument("-output", default="")
    parsed, _remaining = parser.parse_known_args(args)

    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    try:
        if parsed.use_ai:
            cfg = load_config()
            provider = new_provider(cfg)
            healthy, health_msg = provider.is_healthy()
            if not healthy:
                print(f"LLM provider not available: {health_msg}", file=sys.stderr)
                return 1
            chain = Chain(indexer, provider)
            content = chain.generate_project_docs(parsed.prompt)
        else:
            content = _standard_docs(indexer)
    except Exception as err:  # noqa: BLE001
        print(f"error generating docs: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()

    if parsed.output:
        Path(parsed.output).write_text(content, encoding="utf-8")
        print(f"Documentation written to {parsed.output}", file=sys.stderr)
        return 0

    print(content)
    return 0


def handle_stats(graph_db: str, backend: str, verbose: bool) -> int:
    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    try:
        stats = indexer.get_stats()
    except Exception as err:  # noqa: BLE001
        print(f"error getting stats: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()

    _print_stats_summary(stats)
    return 0


def handle_web(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    port = "8080"
    if args:
        port = args[0]

    try:
        import uvicorn
    except Exception as err:  # noqa: BLE001
        print(f"web server requires uvicorn: {err}", file=sys.stderr)
        return 1

    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    app = create_app(indexer)
    host = "127.0.0.1"
    print(f"Starting graph UI at http://localhost:{port}")
    print(f"MCP HTTP endpoint available at http://localhost:{port}/mcp")
    print("Press Ctrl+C to stop")
    try:
        uvicorn.run(app, host=host, port=int(port), log_level="info")
        return 0
    except Exception as err:  # noqa: BLE001
        print(f"web server error: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()


def handle_mcp(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    parser = argparse.ArgumentParser(prog="codecontext mcp", add_help=False)
    parser.add_argument("-http", action="store_true")
    parser.add_argument("-addr", default=":8081")
    parsed, remaining = parser.parse_known_args(args)
    if remaining:
        print(f"unexpected mcp args: {' '.join(remaining)}", file=sys.stderr)
        return 1

    if parsed.http:
        print(f"MCP HTTP server listening on http://localhost{parsed.addr}", file=sys.stderr)
        print("  POST /mcp", file=sys.stderr)
        try:
            return run_http(graph_db, parsed.addr, backend, verbose)
        except Exception as err:  # noqa: BLE001
            print(f"MCP HTTP server error: {err}", file=sys.stderr)
            return 1

    try:
        return run_stdio(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"MCP stdio server error: {err}", file=sys.stderr)
        return 1


def handle_ai(graph_db: str, backend: str, args: list[str], verbose: bool) -> int:
    if not args:
        print("usage: codecontext ai <subcommand> [args]", file=sys.stderr)
        print("subcommands: query, analyze, docs, review, summarize, chat", file=sys.stderr)
        return 1

    subcommand = args[0]
    subargs = args[1:]

    try:
        database, indexer = _open_indexer(graph_db, backend, verbose)
    except Exception as err:  # noqa: BLE001
        print(f"error opening database: {err}", file=sys.stderr)
        return 1
    try:
        cfg = load_config()
        provider = new_provider(cfg)
        healthy, health_msg = provider.is_healthy()
        if not healthy:
            print(f"LLM provider is not available: {health_msg}", file=sys.stderr)
            return 1

        chain = Chain(indexer, provider)

        if subcommand == "query":
            if not subargs:
                print("usage: codecontext ai query <question>", file=sys.stderr)
                return 1
            question = " ".join(subargs)
            print(chain.query_natural(question))
            return 0

        if subcommand == "analyze":
            if not subargs:
                print("usage: codecontext ai analyze <entity_name>", file=sys.stderr)
                return 1
            print(chain.analyze_entity(subargs[0]))
            return 0

        if subcommand == "docs":
            if not subargs:
                print("usage: codecontext ai docs <entity_name>", file=sys.stderr)
                return 1
            print(chain.generate_docs(subargs[0]))
            return 0

        if subcommand == "review":
            if not subargs:
                print("usage: codecontext ai review <entity_name>", file=sys.stderr)
                return 1
            print(chain.review_code(subargs[0]))
            return 0

        if subcommand == "summarize":
            if not subargs:
                print("usage: codecontext ai summarize <file_path>", file=sys.stderr)
                return 1
            print(chain.summarize(subargs[0]))
            return 0

        if subcommand == "chat":
            _handle_ai_chat(chain)
            return 0

        print(f"unknown AI subcommand: {subcommand}", file=sys.stderr)
        return 1
    except Exception as err:  # noqa: BLE001
        print(f"error: {err}", file=sys.stderr)
        return 1
    finally:
        database.close()


def _handle_ai_chat(chain: Chain) -> None:
    print("AI Chat - interactive conversation about code")
    print("Commands: analyze <entity>  docs <entity>  review <entity>  exit")
    conversation = ConversationContext(
        messages=[
            ConversationMessage(
                role="system",
                content=(
                    "You are a helpful code analysis assistant. You have access to a codebase and can help analyze, explain, and improve code."
                ),
            )
        ]
    )

    while True:
        try:
            raw = input("> ").strip()
        except EOFError:
            print("Goodbye")
            return

        if raw == "exit":
            print("Goodbye")
            return

        parts = raw.split()
        handled = False
        if parts:
            if parts[0] in {"analyze", "docs", "review"}:
                if len(parts) < 2:
                    print(f"usage: {parts[0]} <entity_name>")
                    handled = True
                else:
                    name = parts[1]
                    if parts[0] == "analyze":
                        print(chain.analyze_entity(name))
                    elif parts[0] == "docs":
                        print(chain.generate_docs(name))
                    else:
                        print(chain.review_code(name))
                    handled = True

        if not handled:
            print(chain.chat(conversation, raw))


def run(argv: list[str] | None = None) -> int:
    _configure_stdio()

    if argv is None:
        argv = sys.argv[1:]

    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("-ext", default="")
    parser.add_argument("-graph", default=".codecontext.db")
    parser.add_argument("-backend", default=_default_backend())
    parser.add_argument("-version", action="store_true")
    parser.add_argument("-verbose", action="store_true")
    parser.add_argument("-help", action="store_true")

    parsed, remaining = parser.parse_known_args(argv)

    if parsed.help:
        print(USAGE, file=sys.stderr)
        return 0

    if parsed.version:
        print(f"codecontext {__version__}")
        return 0

    if not remaining:
        return legacy_aggregate(".", parsed.ext)

    command = remaining[0]
    cmd_args = remaining[1:]

    if command == "index":
        return handle_index(parsed.graph, parsed.backend, cmd_args, parsed.verbose)
    if command == "query":
        return handle_query(parsed.graph, parsed.backend, cmd_args, parsed.verbose)
    if command == "ai":
        return handle_ai(parsed.graph, parsed.backend, cmd_args, parsed.verbose)
    if command == "docs":
        return handle_docs(parsed.graph, parsed.backend, cmd_args, parsed.verbose)
    if command == "stats":
        return handle_stats(parsed.graph, parsed.backend, parsed.verbose)
    if command == "web":
        return handle_web(parsed.graph, parsed.backend, cmd_args, parsed.verbose)
    if command == "mcp":
        return handle_mcp(parsed.graph, parsed.backend, cmd_args, parsed.verbose)

    exit_code = legacy_aggregate(command, parsed.ext)
    for arg in cmd_args:
        next_code = legacy_aggregate(arg, parsed.ext)
        if next_code != 0:
            exit_code = next_code
    return exit_code
