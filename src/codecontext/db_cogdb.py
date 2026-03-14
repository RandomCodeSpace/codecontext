from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .db import Dependency, Entity, EntityRelation, File


class CogDatabase:
    """Graph database backend using CogDB (pure Python, cross-platform).

    Performance optimizations:
    - flush_interval=0: defers disk flushes to explicit commit()/close() calls
    - put_batch(): groups multiple triples into single bulk writes
    - In-memory dedup caches: avoids scanning CogDB on every insert
    """

    def __init__(self, graph: Any, db_path: str):
        self._g = graph
        self._db_path = db_path
        self._next_file_id = 1
        self._next_entity_id = 1
        self._next_dep_id = 1
        self._next_rel_id = 1
        # Write buffer: triples collected between commit() calls
        self._write_buf: list[tuple[str, str, str]] = []
        # In-memory caches to avoid redundant CogDB queries during indexing
        self._file_path_cache: dict[str, int | None] = {}  # path -> file_id or None
        self._entity_cache: dict[tuple[int, str, str, int], int] = {}  # (file_id, name, type, start_line) -> entity_id
        self._dep_cache: dict[tuple[int, str, str, int], int] = {}  # (source_file_id, target_path, dep_type, line_number) -> dep_id
        self._rel_cache: dict[tuple[int, int, str], int] = {}  # (source_entity_id, target_entity_id, relation_type) -> rel_id

    @classmethod
    def open(cls, db_path: str, verbose: bool = False) -> "CogDatabase":
        _ = verbose
        try:
            from cog.torque import Graph
        except Exception as err:  # noqa: BLE001
            raise RuntimeError(f"failed to import cogdb: {err}") from err

        path = Path(db_path)
        if path.parent and str(path.parent) not in ("", "."):
            path.parent.mkdir(parents=True, exist_ok=True)

        cog_prefix = str(path.parent) if str(path.parent) not in ("", ".") else "."
        cog_home = path.stem + "_cog"

        graph = Graph(
            "codecontext",
            cog_home=cog_home,
            cog_path_prefix=cog_prefix,
            flush_interval=0,
        )
        instance = cls(graph, db_path)
        instance._init_counters()
        return instance

    def close(self) -> None:
        self._flush_buf()
        self._save_counters()
        self._g.sync()
        self._g.close()

    def commit(self) -> None:
        self._flush_buf()
        self._g.sync()

    def _flush_buf(self) -> None:
        if self._write_buf:
            self._g.put_batch(self._write_buf)
            self._write_buf.clear()

    def _buf_put(self, v1: str, pred: str, v2: str) -> None:
        self._write_buf.append((v1, pred, v2))

    # ── Counter management ──────────────────────────────────────────

    def _init_counters(self) -> None:
        self._next_file_id = self._load_counter("file")
        self._next_entity_id = self._load_counter("entity")
        self._next_dep_id = self._load_counter("dep")
        self._next_rel_id = self._load_counter("rel")

    def _load_counter(self, kind: str) -> int:
        r = self._g.v(f"_counter:{kind}").out("val").all()
        if r and r.get("result"):
            return int(r["result"][0]["id"]) + 1
        return 1

    def _save_counters(self) -> None:
        for kind, val in [
            ("file", self._next_file_id - 1),
            ("entity", self._next_entity_id - 1),
            ("dep", self._next_dep_id - 1),
            ("rel", self._next_rel_id - 1),
        ]:
            old = self._g.v(f"_counter:{kind}").out("val").all()
            if old and old.get("result"):
                self._g.delete(f"_counter:{kind}", "val", old["result"][0]["id"])
            self._g.put(f"_counter:{kind}", "val", str(val))

    def _alloc_id(self, kind: str) -> int:
        if kind == "file":
            val = self._next_file_id
            self._next_file_id += 1
        elif kind == "entity":
            val = self._next_entity_id
            self._next_entity_id += 1
        elif kind == "dep":
            val = self._next_dep_id
            self._next_dep_id += 1
        else:
            val = self._next_rel_id
            self._next_rel_id += 1
        return val

    # ── Helpers ─────────────────────────────────────────────────────

    def _put_data(self, node_id: str, data: dict[str, Any]) -> None:
        self._buf_put(node_id, "data", json.dumps(data, separators=(",", ":")))

    def _get_data(self, node_id: str) -> dict[str, Any] | None:
        self._flush_buf()
        r = self._g.v(node_id).out("data").all()
        if not r or not r.get("result"):
            return None
        return json.loads(r["result"][0]["id"])

    def _delete_data(self, node_id: str) -> None:
        r = self._g.v(node_id).out("data").all()
        if r and r.get("result"):
            self._g.delete(node_id, "data", r["result"][0]["id"])

    def _get_refs(self, index_key: str) -> list[str]:
        self._flush_buf()
        r = self._g.v(index_key).out("ref").all()
        if not r or not r.get("result"):
            return []
        return [item["id"] for item in r["result"]]

    def _add_ref(self, index_key: str, node_id: str) -> None:
        self._buf_put(index_key, "ref", node_id)

    def _del_ref(self, index_key: str, node_id: str) -> None:
        self._g.delete(index_key, "ref", node_id)

    # ── File operations ─────────────────────────────────────────────

    def insert_file(self, path: str, language: str, file_hash: str, lines_of_code: int, tokens: int) -> int:
        cached_id = self._file_path_cache.get(path)
        if cached_id is not None:
            node_id = f"file:{cached_id}"
            self._flush_buf()
            data = self._get_data(node_id)
            if data is not None:
                data.update(language=language, hash=file_hash, lines_of_code=lines_of_code, tokens=tokens)
                self._delete_data(node_id)
                self._put_data(node_id, data)
                return cached_id

        existing = self._find_file_node(path)
        if existing is not None:
            node_id, data = existing
            data.update(language=language, hash=file_hash, lines_of_code=lines_of_code, tokens=tokens)
            self._delete_data(node_id)
            self._put_data(node_id, data)
            fid = int(data["id"])
            self._file_path_cache[path] = fid
            return fid

        file_id = self._alloc_id("file")
        node_id = f"file:{file_id}"
        data = {
            "id": file_id,
            "path": path,
            "language": language,
            "hash": file_hash,
            "lines_of_code": lines_of_code,
            "tokens": tokens,
        }
        self._put_data(node_id, data)
        self._add_ref(f"_idx:fp:{path}", node_id)
        self._add_ref("_all:files", node_id)
        self._file_path_cache[path] = file_id
        return file_id

    def _find_file_node(self, path: str) -> tuple[str, dict[str, Any]] | None:
        self._flush_buf()
        refs = self._get_refs(f"_idx:fp:{path}")
        if not refs:
            self._file_path_cache[path] = None  # negative cache
            return None
        node_id = refs[0]
        data = self._get_data(node_id)
        if data is None:
            return None
        self._file_path_cache[path] = int(data["id"])
        return node_id, data

    def get_file_by_path(self, path: str) -> File | None:
        cached_id = self._file_path_cache.get(path)
        if cached_id is None and path in self._file_path_cache:
            return None  # negative cache hit
        if cached_id is not None:
            self._flush_buf()
            data = self._get_data(f"file:{cached_id}")
            if data is not None:
                return self._data_to_file(data)
        result = self._find_file_node(path)
        if result is None:
            return None
        return self._data_to_file(result[1])

    def get_file_by_id(self, file_id: int) -> File | None:
        data = self._get_data(f"file:{file_id}")
        if data is None:
            return None
        return self._data_to_file(data)

    def get_files(self) -> list[File]:
        self._flush_buf()
        refs = self._get_refs("_all:files")
        files = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                f = self._data_to_file(data)
                files.append(f)
                self._file_path_cache[f.path] = f.id
        return files

    @staticmethod
    def _data_to_file(data: dict[str, Any]) -> File:
        return File(
            id=int(data["id"]),
            path=data["path"],
            language=data["language"],
            hash=data.get("hash", ""),
            lines_of_code=int(data.get("lines_of_code", 0)),
            tokens=int(data.get("tokens", 0)),
        )

    # ── Entity operations ───────────────────────────────────────────

    def insert_entity(
        self,
        file_id: int,
        name: str,
        entity_type: str,
        kind: str,
        signature: str,
        start_line: int,
        end_line: int,
        docs: str,
        parent: str,
        visibility: str,
        scope: str,
        language: str,
    ) -> int:
        cache_key = (file_id, name, entity_type, start_line)
        cached = self._entity_cache.get(cache_key)
        if cached is not None:
            return cached

        entity_id = self._alloc_id("entity")
        node_id = f"entity:{entity_id}"
        data = {
            "id": entity_id,
            "file_id": file_id,
            "name": name,
            "type": entity_type,
            "kind": kind,
            "signature": signature,
            "start_line": start_line,
            "end_line": end_line,
            "column_start": 0,
            "column_end": 0,
            "documentation": docs,
            "parent": parent,
            "visibility": visibility,
            "scope": scope,
            "language": language,
            "attributes": "",
        }
        self._put_data(node_id, data)
        self._add_ref(f"_idx:ef:{file_id}", node_id)
        self._add_ref(f"_idx:en:{name}", node_id)
        self._add_ref("_all:entities", node_id)
        self._entity_cache[cache_key] = entity_id
        return entity_id

    def get_entities_by_file(self, file_id: int) -> list[Entity]:
        refs = self._get_refs(f"_idx:ef:{file_id}")
        entities = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                entities.append(self._data_to_entity(data))
        return entities

    def get_entity_by_name(self, name: str) -> list[Entity]:
        refs = self._get_refs(f"_idx:en:{name}")
        entities = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                entities.append(self._data_to_entity(data))
        return entities

    def get_entity_by_id(self, entity_id: int) -> Entity | None:
        data = self._get_data(f"entity:{entity_id}")
        if data is None:
            return None
        return self._data_to_entity(data)

    def get_all_entities(self) -> list[Entity]:
        refs = self._get_refs("_all:entities")
        entities = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                entities.append(self._data_to_entity(data))
        return entities

    def delete_entities_by_file(self, file_id: int) -> None:
        refs = self._get_refs(f"_idx:ef:{file_id}")
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                name = data["name"]
                entity_type = data["type"]
                start_line = int(data["start_line"])
                entity_id = int(data["id"])
                self._del_ref(f"_idx:en:{name}", node_id)
                self._del_ref("_all:entities", node_id)
                self._delete_relations_for_entity(entity_id)
                self._entity_cache.pop((file_id, name, entity_type, start_line), None)
            self._delete_data(node_id)
            self._del_ref(f"_idx:ef:{file_id}", node_id)

    def _delete_relations_for_entity(self, entity_id: int) -> None:
        for prefix in ("_idx:rs:", "_idx:rt:"):
            refs = self._get_refs(f"{prefix}{entity_id}")
            for node_id in refs:
                data = self._get_data(node_id)
                if data is not None:
                    src = int(data["source_entity_id"])
                    tgt = int(data["target_entity_id"])
                    rtype = data["relation_type"]
                    self._del_ref(f"_idx:rs:{src}", node_id)
                    self._del_ref(f"_idx:rt:{tgt}", node_id)
                    self._del_ref("_all:relations", node_id)
                    self._rel_cache.pop((src, tgt, rtype), None)
                self._delete_data(node_id)

    @staticmethod
    def _data_to_entity(data: dict[str, Any]) -> Entity:
        return Entity(
            id=int(data["id"]),
            file_id=int(data["file_id"]),
            name=data["name"],
            type=data["type"],
            kind=data.get("kind", ""),
            signature=data.get("signature", ""),
            start_line=int(data.get("start_line", 0)),
            end_line=int(data.get("end_line", 0)),
            column_start=int(data.get("column_start", 0)),
            column_end=int(data.get("column_end", 0)),
            documentation=data.get("documentation", ""),
            parent=data.get("parent", ""),
            visibility=data.get("visibility", ""),
            scope=data.get("scope", ""),
            language=data.get("language", ""),
            attributes=data.get("attributes", ""),
        )

    # ── Dependency operations ───────────────────────────────────────

    def insert_dependency(self, source_file_id: int, target_path: str, dep_type: str, line_number: int) -> int:
        cache_key = (source_file_id, target_path, dep_type, line_number)
        cached = self._dep_cache.get(cache_key)
        if cached is not None:
            return cached

        dep_id = self._alloc_id("dep")
        node_id = f"dep:{dep_id}"
        data = {
            "id": dep_id,
            "source_file_id": source_file_id,
            "target_path": target_path,
            "import_type": dep_type,
            "line_number": line_number,
            "resolved": "",
            "is_local": False,
        }
        self._put_data(node_id, data)
        self._add_ref(f"_idx:df:{source_file_id}", node_id)
        self._add_ref("_all:deps", node_id)
        self._dep_cache[cache_key] = dep_id
        return dep_id

    def get_dependencies(self, file_id: int) -> list[Dependency]:
        refs = self._get_refs(f"_idx:df:{file_id}")
        deps = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                deps.append(self._data_to_dep(data))
        return deps

    def get_all_dependencies(self) -> list[Dependency]:
        refs = self._get_refs("_all:deps")
        deps = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                deps.append(self._data_to_dep(data))
        return deps

    def delete_dependencies_by_file(self, file_id: int) -> None:
        refs = self._get_refs(f"_idx:df:{file_id}")
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                cache_key = (
                    int(data["source_file_id"]),
                    data["target_path"],
                    data["import_type"],
                    int(data["line_number"]),
                )
                self._dep_cache.pop(cache_key, None)
            self._delete_data(node_id)
            self._del_ref(f"_idx:df:{file_id}", node_id)
            self._del_ref("_all:deps", node_id)

    @staticmethod
    def _data_to_dep(data: dict[str, Any]) -> Dependency:
        return Dependency(
            id=int(data["id"]),
            source_file_id=int(data["source_file_id"]),
            target_path=data["target_path"],
            import_type=data.get("import_type", ""),
            line_number=int(data.get("line_number", 0)),
            resolved=data.get("resolved", ""),
            is_local=bool(data.get("is_local", False)),
        )

    # ── Entity relation operations ──────────────────────────────────

    def insert_entity_relation(
        self,
        source_entity_id: int,
        target_entity_id: int,
        relation_type: str,
        line_number: int,
        context: str,
    ) -> int:
        cache_key = (source_entity_id, target_entity_id, relation_type)
        cached = self._rel_cache.get(cache_key)
        if cached is not None:
            return cached

        rel_id = self._alloc_id("rel")
        node_id = f"rel:{rel_id}"
        data = {
            "id": rel_id,
            "source_entity_id": source_entity_id,
            "target_entity_id": target_entity_id,
            "relation_type": relation_type,
            "line_number": line_number,
            "context": context,
        }
        self._put_data(node_id, data)
        self._add_ref(f"_idx:rs:{source_entity_id}", node_id)
        self._add_ref(f"_idx:rt:{target_entity_id}", node_id)
        self._add_ref("_all:relations", node_id)
        self._rel_cache[cache_key] = rel_id
        return rel_id

    def get_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        refs = self._get_refs(f"_idx:rs:{entity_id}")
        relations = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is None:
                continue
            if relation_type and data["relation_type"] != relation_type:
                continue
            relations.append(self._data_to_relation(data))
        return relations

    def get_inbound_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        refs = self._get_refs(f"_idx:rt:{entity_id}")
        relations = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is None:
                continue
            if relation_type and data["relation_type"] != relation_type:
                continue
            relations.append(self._data_to_relation(data))
        return relations

    def get_all_relations(self) -> list[EntityRelation]:
        refs = self._get_refs("_all:relations")
        relations = []
        for node_id in refs:
            data = self._get_data(node_id)
            if data is not None:
                relations.append(self._data_to_relation(data))
        return relations

    @staticmethod
    def _data_to_relation(data: dict[str, Any]) -> EntityRelation:
        return EntityRelation(
            id=int(data["id"]),
            source_entity_id=int(data["source_entity_id"]),
            target_entity_id=int(data["target_entity_id"]),
            relation_type=data["relation_type"],
            line_number=int(data.get("line_number", 0)),
            context=data.get("context", ""),
        )

    # ── Search and graph queries ────────────────────────────────────

    def search_entities(self, query: str, limit: int = 20) -> list[Entity]:
        q = query.lower()
        refs = self._get_refs("_all:entities")
        results = []
        for node_id in refs:
            if len(results) >= limit:
                break
            data = self._get_data(node_id)
            if data is None:
                continue
            if q in data.get("name", "").lower() or q in data.get("type", "").lower() or q in data.get("parent", "").lower():
                results.append(self._data_to_entity(data))
        results.sort(key=lambda e: (e.name, e.id))
        return results[:limit]

    def get_file_imports(self, path: str) -> list[str]:
        file = self.get_file_by_path(path)
        if file is None:
            return []
        deps = self.get_dependencies(file.id)
        deps.sort(key=lambda d: (d.line_number, d.id))
        return [d.target_path for d in deps]

    def get_call_graph(self, entity_id: int, depth: int = 1) -> dict[str, Any]:
        _ = depth
        entity = self.get_entity_by_id(entity_id)
        if entity is None:
            raise ValueError("entity not found")

        relations = self.get_entity_relations(entity_id, "calls")
        calls: list[dict[str, Any]] = []
        for rel in relations:
            target = self.get_entity_by_id(rel.target_entity_id)
            if target is None:
                continue
            calls.append({"id": target.id, "name": target.name, "type": target.type})

        return {
            "entity": {"id": entity.id, "name": entity.name, "type": entity.type},
            "calls": calls,
        }

    def get_dependency_graph(self, file_path: str) -> dict[str, Any]:
        file = self.get_file_by_path(file_path)
        if file is None:
            raise ValueError(f"file not found: {file_path}")
        deps = self.get_dependencies(file.id)
        return {"file": file.path, "dependencies": [dep.target_path for dep in deps]}

    # ── Metrics ─────────────────────────────────────────────────────

    def get_file_count(self) -> int:
        return len(self._get_refs("_all:files"))

    def get_lines_of_code_count(self) -> int:
        total = 0
        for node_id in self._get_refs("_all:files"):
            data = self._get_data(node_id)
            if data:
                total += int(data.get("lines_of_code", 0))
        return total

    def get_tokens_count(self) -> int:
        total = 0
        for node_id in self._get_refs("_all:files"):
            data = self._get_data(node_id)
            if data:
                total += int(data.get("tokens", 0))
        return total

    def get_entity_count(self) -> int:
        return len(self._get_refs("_all:entities"))

    def get_dependency_count(self) -> int:
        return len(self._get_refs("_all:deps"))

    def get_relation_count(self) -> int:
        return len(self._get_refs("_all:relations"))
