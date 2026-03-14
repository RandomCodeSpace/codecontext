from __future__ import annotations

from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

from fastapi import FastAPI, Query
from fastapi.responses import HTMLResponse, Response

from .db import Dependency, Entity, EntityRelation, File
from .indexer import Indexer
from .mcp import MCPService


@dataclass
class GraphNode:
	id: str
	label: str
	type: str
	group: str
	filePath: str = ""
	parent: str = ""
	line: int = 0


@dataclass
class GraphEdge:
	source: str
	target: str
	type: str


def create_app(indexer: Indexer) -> FastAPI:
	app = FastAPI(title="codecontext-web", version="0.1")
	mcp_service = MCPService(indexer.db, indexer)

	@app.get("/")
	async def root() -> HTMLResponse:
		return HTMLResponse(content=_ui_html())

	@app.get("/favicon.ico")
	async def favicon() -> Response:
		# Explicit empty favicon response to avoid noisy 404 logs.
		return Response(status_code=204)

	@app.get("/api/stats")
	async def stats() -> dict[str, int]:
		return indexer.get_stats()

	@app.get("/api/graph")
	async def graph() -> dict[str, list[dict[str, object]]]:
		files = indexer.get_all_files()
		entities = indexer.get_all_entities()
		relations = indexer.get_all_relations()
		deps = indexer.get_all_dependencies()

		nodes: list[GraphNode] = []
		edges: list[GraphEdge] = []

		file_id_to_node: dict[int, str] = {}
		for file in files:
			nid = f"f-{file.id}"
			file_id_to_node[file.id] = nid
			nodes.append(
				GraphNode(
					id=nid,
					label=_short_path(file.path),
					type=file.language,
					group="file",
					filePath=file.path,
				)
			)

		entity_id_to_node: dict[int, str] = {}
		for entity in entities:
			nid = f"e-{entity.id}"
			entity_id_to_node[entity.id] = nid
			nodes.append(
				GraphNode(
					id=nid,
					label=entity.name,
					type=entity.type,
					group="entity",
					filePath=file_id_to_node.get(entity.file_id, ""),
					parent=entity.parent,
					line=entity.start_line,
				)
			)
			parent_file_node = file_id_to_node.get(entity.file_id)
			if parent_file_node:
				edges.append(GraphEdge(source=parent_file_node, target=nid, type="contains"))

		for rel in relations:
			source = entity_id_to_node.get(rel.source_entity_id)
			target = entity_id_to_node.get(rel.target_entity_id)
			if source and target:
				edges.append(GraphEdge(source=source, target=target, type=rel.relation_type))

		files_by_base: dict[str, list[File]] = {}
		for file in files:
			files_by_base.setdefault(Path(file.path).name, []).append(file)

		for dep in deps:
			source_file_node = file_id_to_node.get(dep.source_file_id)
			if not source_file_node:
				continue
			target_file = _resolve_dep(dep, files_by_base)
			if target_file is None:
				continue
			target_node = file_id_to_node.get(target_file.id)
			if target_node:
				edges.append(GraphEdge(source=source_file_node, target=target_node, type="imports"))

		return {
			"nodes": [asdict(n) for n in nodes],
			"edges": [asdict(e) for e in edges],
		}

	@app.get("/api/tree")
	async def tree() -> dict[str, object]:
		files = indexer.get_all_files()
		return _build_tree(files)

	@app.get("/api/dir")
	async def dir_detail(path: str = Query(default="")) -> dict[str, object]:
		files = indexer.get_all_files()
		deps = indexer.get_all_dependencies()
		file_by_id = {f.id: f for f in files}

		dir_files = [f for f in files if _is_under_path(f.path, path)]
		dir_file_ids = {f.id for f in dir_files}

		imported_from: set[str] = set()
		imported_by: set[str] = set()
		base_to_files: dict[str, list[File]] = {}
		for file in files:
			base_to_files.setdefault(Path(file.path).name, []).append(file)

		for dep in deps:
			target_file = _resolve_dep(dep, base_to_files)
			if dep.source_file_id in dir_file_ids:
				if target_file and target_file.id not in dir_file_ids:
					imported_from.add(_dir_of_path(target_file.path))
			elif target_file and target_file.id in dir_file_ids:
				source_file = file_by_id.get(dep.source_file_id)
				if source_file:
					imported_by.add(_dir_of_path(source_file.path))

		top_files = [Path(f.path).name for f in dir_files[:20]]
		top_entities: list[dict[str, str]] = []
		for file in dir_files[:10]:
			for entity in indexer.get_entities_by_file(file.id):
				if len(top_entities) >= 30:
					break
				top_entities.append({"name": entity.name, "type": entity.type, "file": Path(file.path).name})
			if len(top_entities) >= 30:
				break

		return {
			"path": path,
			"fileCount": len(dir_files),
			"importsFrom": sorted(imported_from),
			"importedBy": sorted(imported_by),
			"topFiles": top_files,
			"topEntities": top_entities,
		}

	@app.post("/mcp")
	async def mcp_endpoint(payload: dict[str, Any]) -> dict[str, Any]:
		request_id = payload.get("id")
		tool = str(payload.get("tool", ""))
		arguments = payload.get("arguments", {})
		try:
			result = mcp_service.call_tool(tool, arguments)
			return {"id": request_id, "result": result}
		except Exception as err:  # noqa: BLE001
			return {"id": request_id, "error": str(err)}

	return app


def _ui_html() -> str:
	ui_path = Path(__file__).with_name("ui.html")
	return ui_path.read_text(encoding="utf-8")


def _resolve_dep(dep: Dependency, files_by_base: dict[str, list[File]]) -> File | None:
	target = dep.target_path.replace(".", "/")
	base = Path(target).name
	if not base:
		return None
	candidates = [base]
	if "." not in base:
		candidates.extend([f"{base}.go", f"{base}.py", f"{base}.js", f"{base}.ts", f"{base}.java"])
	for cand in candidates:
		for file in files_by_base.get(cand, []):
			if _path_suffix_match(file.path, target):
				return file
	return None


def _path_suffix_match(file_path: str, import_path: str) -> bool:
	value = import_path
	while value.startswith(".") or value.startswith("/"):
		value = value[1:]
	if not value:
		return False
	suffixes = [value, f"{value}.go", f"{value}.py", f"{value}.js", f"{value}.ts", f"{value}.java"]
	return any(file_path.endswith(sfx) for sfx in suffixes)


def _short_path(path: str) -> str:
	parts = [p for p in path.replace("\\", "/").split("/") if p]
	if len(parts) <= 2:
		return path
	return f"{parts[-2]}/{parts[-1]}"


def _build_tree(files: list[File]) -> dict[str, object]:
	root: dict[str, object] = {"name": ".", "path": "", "count": 0, "children": {}, "lang": ""}

	for file in files:
		parts = [p for p in file.path.replace("\\", "/").split("/") if p]
		cur = root
		cur["count"] = int(cur["count"]) + 1
		for i, part in enumerate(parts):
			children: dict[str, dict[str, object]] = cur.setdefault("children", {})  # type: ignore[assignment]
			if part not in children:
				path = part if not cur.get("path") else f"{cur['path']}/{part}"
				children[part] = {"name": part, "path": path, "count": 0, "children": {}, "lang": ""}
			child = children[part]
			child["count"] = int(child["count"]) + 1
			if i == len(parts) - 1:
				child["lang"] = file.language
			cur = child

	def dominant_lang(node: dict[str, object]) -> str:
		lang = str(node.get("lang") or "")
		if lang:
			return lang

		counts: dict[str, int] = {}
		children_map = node.get("children") or {}
		if not isinstance(children_map, dict):
			return ""

		for child in children_map.values():
			if not isinstance(child, dict):
				continue
			child_lang = dominant_lang(child)
			if not child_lang:
				continue
			counts[child_lang] = counts.get(child_lang, 0) + int(child.get("count", 0))

		if not counts:
			return ""
		best_lang = max(counts.items(), key=lambda item: item[1])[0]
		node["lang"] = best_lang
		return best_lang

	dominant_lang(root)

	def finalize(node: dict[str, object]) -> dict[str, object]:
		raw_children = list((node.get("children") or {}).values())
		children = [finalize(child) for child in raw_children]
		children.sort(key=lambda c: int(c.get("count", 0)), reverse=True)
		out = {
			"name": node.get("name", ""),
			"path": node.get("path", ""),
			"count": int(node.get("count", 0)),
			"lang": node.get("lang", ""),
		}
		if children:
			out["children"] = children
		return out

	return finalize(root)


def _is_under_path(file_path: str, dir_path: str) -> bool:
	if not dir_path or dir_path == ".":
		return True
	return file_path == dir_path or file_path.startswith(f"{dir_path}/") or file_path.startswith(f"{dir_path}\\")


def _dir_of_path(path: str) -> str:
	parts = [p for p in path.replace("\\", "/").split("/") if p]
	if len(parts) <= 1:
		return "."
	return "/".join(parts[:-1])
