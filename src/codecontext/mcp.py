from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any

from .backends import open_backend
from .indexer import Indexer
from .storage import StorageBackend


class MCPService:
	def __init__(self, database: StorageBackend, indexer: Indexer):
		self.db = database
		self.indexer = indexer

	def call_tool(self, tool: str, arguments: dict[str, Any] | None = None) -> dict[str, Any]:
		args = arguments or {}

		if tool == "index_directory":
			path = str(args.get("path", ""))
			if not path:
				raise ValueError("path is required")
			self.indexer.index_directory(path)
			return {"status": "ok", "path": path}

		if tool == "query_entity":
			name = str(args.get("name", ""))
			if not name:
				raise ValueError("name is required")
			entities = self.indexer.query_entity(name)
			return {
				"count": len(entities),
				"name": name,
				"results": [_entity_result(self.db, entity) for entity in entities],
			}

		if tool == "query_call_graph":
			entity_id = _int_arg(args, "entity_id")
			return self.indexer.query_call_graph(entity_id)

		if tool == "query_dependencies":
			path = str(args.get("path", ""))
			if not path:
				raise ValueError("path is required")
			return self.indexer.query_dependency_graph(path)

		if tool == "graph_stats":
			return self.indexer.get_stats()

		if tool == "list_files":
			language = str(args.get("language", "")).strip().lower()
			files = self.db.get_files()
			if language:
				normalized = language.lstrip(".")
				files = [f for f in files if f.language.lower() == normalized]
			rows = [{"path": f.path, "language": f.language} for f in files]
			return {"count": len(rows), "files": rows}

		if tool == "get_file_outline":
			path = str(args.get("path", ""))
			if not path:
				raise ValueError("path is required")
			file = self.db.get_file_by_path(path)
			if file is None:
				raise ValueError(f"file not found: {path}")
			entities = self.db.get_entities_by_file(file.id)
			rows = [
				{
					"id": e.id,
					"qualified_name": f"{e.parent}.{e.name}" if e.parent else e.name,
					"type": e.type,
					"start_line": e.start_line,
					"end_line": e.end_line,
				}
				for e in entities
			]
			return {"count": len(rows), "file": path, "entities": rows}

		if tool == "find_usages":
			entity_id = _int_arg(args, "entity_id")
			rels = self.db.get_inbound_entity_relations(entity_id)
			callers = []
			for rel in rels:
				source = self.db.get_entity_by_id(rel.source_entity_id)
				if source is None:
					continue
				callers.append(
					{
						"id": source.id,
						"qualified_name": f"{source.parent}.{source.name}" if source.parent else source.name,
						"type": source.type,
						"relation_type": rel.relation_type,
						"line_number": rel.line_number,
					}
				)
			return {"entity_id": entity_id, "count": len(callers), "callers": callers}

		if tool == "search_entities":
			query = str(args.get("query", ""))
			if not query:
				raise ValueError("query is required")
			limit = int(args.get("limit", 20))
			entities = self.db.search_entities(query, limit)
			rows = [_entity_result(self.db, entity) for entity in entities]
			return {"query": query, "count": len(rows), "results": rows}

		if tool == "get_entity_code":
			entity_id = _int_arg(args, "entity_id")
			entity = self.db.get_entity_by_id(entity_id)
			if entity is None:
				raise ValueError("entity not found")
			file = self.db.get_file_by_id(entity.file_id)
			if file is None:
				raise ValueError("file not found")
			code = _slice_file(file.path, entity.start_line, entity.end_line)
			return {
				"entity_id": entity.id,
				"file": file.path,
				"start_line": entity.start_line,
				"end_line": entity.end_line,
				"code": code,
			}

		if tool == "get_file_imports":
			path = str(args.get("path", ""))
			if not path:
				raise ValueError("path is required")
			imports = self.db.get_file_imports(path)
			return {"path": path, "count": len(imports), "imports": imports}

		if tool == "get_docs":
			scope = str(args.get("scope", "entity") or "entity")
			name = str(args.get("name", ""))
			if scope == "project":
				stats = self.indexer.get_stats()
				return {
					"scope": "project",
					"summary": f"Project has {stats['files']} files and {stats['entities']} entities.",
				}
			if scope == "file":
				if not name:
					raise ValueError("name is required for file scope")
				file = self.db.get_file_by_path(name)
				if file is None:
					raise ValueError(f"file not found: {name}")
				entities = self.db.get_entities_by_file(file.id)
				return {
					"scope": "file",
					"name": name,
					"summary": f"File {name} contains {len(entities)} entities.",
				}
			if not name:
				raise ValueError("name is required for entity scope")
			entities = self.indexer.query_entity(name)
			if not entities:
				return {"scope": "entity", "name": name, "summary": "Entity not found."}
			entity = entities[0]
			return {
				"scope": "entity",
				"name": name,
				"summary": f"{entity.type} {name} at lines {entity.start_line}-{entity.end_line}.",
			}

		raise ValueError(f"unknown tool: {tool}")


def run_stdio(graph_db: str, backend: str = "sqlite", verbose: bool = False) -> int:
	database = open_backend(backend, graph_db, verbose)
	indexer = Indexer(database, backend)
	indexer.set_verbose(verbose)
	service = MCPService(database, indexer)

	print(json.dumps({"level": "INFO", "msg": "mcp stdio server ready", "db": graph_db}), file=sys.stderr)

	try:
		for line in sys.stdin:
			text = line.strip()
			if not text:
				continue
			try:
				request = json.loads(text)
				request_id = request.get("id")
				tool = str(request.get("tool", ""))
				arguments = request.get("arguments", {})
				result = service.call_tool(tool, arguments)
				response = {"id": request_id, "result": result}
			except Exception as err:  # noqa: BLE001
				response = {"id": request.get("id") if isinstance(request, dict) else None, "error": str(err)}
			print(json.dumps(response), flush=True)
	finally:
		database.close()
	return 0


def run_http(graph_db: str, addr: str, backend: str = "sqlite", verbose: bool = False) -> int:
	try:
		from fastapi import FastAPI
		from fastapi.responses import JSONResponse
		import uvicorn
	except Exception as err:  # noqa: BLE001
		raise RuntimeError(f"HTTP MCP mode requires fastapi/uvicorn: {err}") from err

	database = open_backend(backend, graph_db, verbose)
	indexer = Indexer(database, backend)
	indexer.set_verbose(verbose)
	service = MCPService(database, indexer)

	app = FastAPI(title="codecontext-mcp", version="0.1")

	@app.post("/mcp")
	async def mcp_endpoint(payload: dict[str, Any]) -> JSONResponse:
		request_id = payload.get("id")
		tool = str(payload.get("tool", ""))
		arguments = payload.get("arguments", {})
		try:
			result = service.call_tool(tool, arguments)
			return JSONResponse({"id": request_id, "result": result})
		except Exception as err:  # noqa: BLE001
			return JSONResponse({"id": request_id, "error": str(err)}, status_code=400)

	host, port = _split_addr(addr)
	uvicorn.run(app, host=host, port=port, log_level="info")
	database.close()
	return 0


def _entity_result(db: StorageBackend, entity: Any) -> dict[str, Any]:
	file = db.get_file_by_id(entity.file_id)
	return {
		"id": entity.id,
		"name": entity.name,
		"qualified_name": f"{entity.parent}.{entity.name}" if entity.parent else entity.name,
		"type": entity.type,
		"file_id": entity.file_id,
		"file_path": file.path if file else "",
		"start_line": entity.start_line,
		"end_line": entity.end_line,
	}


def _slice_file(path: str, start_line: int, end_line: int) -> str:
	p = Path(path)
	if not p.exists():
		return ""
	lines = p.read_text(encoding="utf-8", errors="replace").splitlines()
	start = max(start_line - 1, 0)
	end = max(end_line, start)
	return "\n".join(lines[start:end])


def _split_addr(addr: str) -> tuple[str, int]:
	value = addr.strip()
	if not value:
		return ("127.0.0.1", 8081)
	if value.startswith(":"):
		return ("127.0.0.1", int(value[1:]))
	m = re.match(r"^([^:]+):(\d+)$", value)
	if m:
		return (m.group(1), int(m.group(2)))
	raise ValueError(f"invalid addr format: {addr}")


def _int_arg(args: dict[str, Any], key: str) -> int:
	if key not in args:
		raise ValueError(f"{key} is required")
	return int(args[key])
