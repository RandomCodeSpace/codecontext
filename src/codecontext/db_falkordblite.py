from __future__ import annotations

import atexit
from dataclasses import dataclass
from pathlib import Path
import threading
from typing import Any

from .db import Dependency, Entity, EntityRelation, File


@dataclass
class _RelRow:
    id: int
    source_id: int
    target_id: int
    relation_type: str
    line_number: int
    context: str


class FalkorLiteDatabase:
    _instances: dict[str, tuple[Any, Any, int]] = {}
    _lock = threading.Lock()

    def __init__(self, db_path: str, db: Any, graph: Any):
        self._db_path = db_path
        self._db = db
        self._g = graph
        self._id_counters: dict[str, int] = {}
        self._entity_key_cache: dict[int, dict[tuple[str, str, int], int]] = {}
        self._dep_key_cache: dict[int, dict[tuple[str, str, int], int]] = {}

    @classmethod
    def open(cls, db_path: str, verbose: bool = False) -> "FalkorLiteDatabase":
        _ = verbose
        try:
            from redislite.falkordb_client import FalkorDB
        except Exception as err:  # noqa: BLE001
            raise RuntimeError(f"failed to import FalkorDBLite: {err}") from err

        path = Path(db_path)
        if path.parent and str(path.parent) not in ("", "."):
            path.parent.mkdir(parents=True, exist_ok=True)

        key = str(path.resolve())
        with cls._lock:
            existing = cls._instances.get(key)
            if existing is not None:
                db, graph, refs = existing
                cls._instances[key] = (db, graph, refs + 1)
                return cls(key, db, graph)

            db = FalkorDB(str(path))
            client = getattr(db, "client", None)
            cleanup = getattr(client, "_cleanup", None)
            if cleanup is not None:
                try:
                    atexit.unregister(cleanup)
                except Exception:
                    pass
            if client is not None:
                # redislite may call cleanup from __del__ during interpreter teardown,
                # where builtins can already be cleared, causing noisy NameError traces.
                try:
                    setattr(client, "_cleanup", lambda *_args, **_kwargs: None)
                except Exception:
                    pass
            graph = db.select_graph("codecontext")
            cls._instances[key] = (db, graph, 1)

        instance = cls(key, db, graph)
        instance._migrate()
        instance._init_counters()
        return instance

    def close(self) -> None:
        # Keep the embedded server alive for process lifetime to avoid close deadlocks.
        with self.__class__._lock:
            item = self.__class__._instances.get(self._db_path)
            if item is None:
                return
            db, graph, refs = item
            if refs <= 1:
                self.__class__._instances[self._db_path] = (db, graph, 0)
            else:
                self.__class__._instances[self._db_path] = (db, graph, refs - 1)

    def commit(self) -> None:
        # FalkorDBLite writes are persisted immediately.
        return

    def _migrate(self) -> None:
        # Initialize graph labels and indexes used by lookups.
        self._safe_query("CREATE INDEX FOR (f:File) ON (f.path)")
        self._safe_query("CREATE INDEX FOR (f:File) ON (f.id)")
        self._safe_query("CREATE INDEX FOR (e:Entity) ON (e.id)")
        self._safe_query("CREATE INDEX FOR (e:Entity) ON (e.name)")
        self._safe_query("CREATE INDEX FOR (d:Dependency) ON (d.id)")

    def _safe_query(self, query: str, params: dict[str, Any] | None = None) -> None:
        try:
            self._g.query(query, params=params or {})
        except Exception:
            # Index creation is idempotent from our perspective; ignore create conflicts.
            pass

    def _query(self, query: str, params: dict[str, Any] | None = None) -> list[list[Any]]:
        res = self._g.query(query, params=params or {})
        return list(getattr(res, "result_set", []) or [])

    def _init_counters(self) -> None:
        self._id_counters["File"] = self._max_node_id("File")
        self._id_counters["Entity"] = self._max_node_id("Entity")
        self._id_counters["Dependency"] = self._max_node_id("Dependency")
        self._id_counters["RELATES"] = self._max_relation_id("RELATES")

    def _max_node_id(self, label: str) -> int:
        rows = self._query(f"MATCH (n:{label}) RETURN coalesce(max(n.id), 0)")
        return int(rows[0][0]) if rows else 0

    def _max_relation_id(self, rel_type: str) -> int:
        rows = self._query(f"MATCH ()-[r:{rel_type}]->() RETURN coalesce(max(r.id), 0)")
        return int(rows[0][0]) if rows else 0

    def _alloc_id(self, key: str) -> int:
        current = int(self._id_counters.get(key, 0)) + 1
        self._id_counters[key] = current
        return current

    @staticmethod
    def _chunked[T](items: list[T], size: int = 500) -> list[list[T]]:
        if size <= 0:
            size = 500
        return [items[idx : idx + size] for idx in range(0, len(items), size)]

    def _existing_file_ids_by_path(self, paths: list[str]) -> dict[str, int]:
        if not paths:
            return {}
        rows: list[list[Any]] = []
        for chunk in self._chunked(paths, 1000):
            rows.extend(
                self._query(
                    """
                    UNWIND $paths AS path
                    MATCH (f:File {path: path})
                    RETURN f.path, f.id
                    """,
                    {"paths": chunk},
                )
            )
        return {str(row[0]): int(row[1]) for row in rows}

    def _delete_graph_for_files(self, file_ids: list[int]) -> None:
        if not file_ids:
            return
        for chunk in self._chunked(file_ids, 1000):
            self._query(
                """
                UNWIND $file_ids AS file_id
                OPTIONAL MATCH (:File {id: file_id})-[:CONTAINS]->(e:Entity)
                DETACH DELETE e
                """,
                {"file_ids": chunk},
            )
            self._query(
                """
                UNWIND $file_ids AS file_id
                OPTIONAL MATCH (:File {id: file_id})-[:HAS_DEP]->(d:Dependency)
                DETACH DELETE d
                """,
                {"file_ids": chunk},
            )
        for file_id in file_ids:
            self._entity_key_cache.pop(file_id, None)
            self._dep_key_cache.pop(file_id, None)

    def bulk_sync_files(self, rows: list[dict[str, Any]]) -> dict[int, int]:
        if not rows:
            return {}

        existing_by_path = self._existing_file_ids_by_path([str(row["path"]) for row in rows])
        existing_file_ids = sorted(set(existing_by_path.values()))
        if existing_file_ids:
            self._delete_graph_for_files(existing_file_ids)

        payload: list[dict[str, Any]] = []
        stage_to_dest: dict[int, int] = {}
        for row in rows:
            stage_id = int(row["stage_id"])
            path = str(row["path"])
            dest_id = existing_by_path.get(path)
            if dest_id is None:
                dest_id = self._alloc_id("File")
            stage_to_dest[stage_id] = dest_id
            payload.append(
                {
                    "dest_id": dest_id,
                    "path": path,
                    "language": str(row["language"]),
                    "hash": str(row["hash"]),
                    "lines_of_code": int(row["lines_of_code"]),
                    "tokens": int(row["tokens"]),
                }
            )

        try:
            for chunk in self._chunked(payload):
                self._query(
                    """
                    UNWIND $rows AS row
                    MERGE (f:File {path: row.path})
                    SET f.id = row.dest_id,
                        f.language = row.language,
                        f.hash = row.hash,
                        f.lines_of_code = row.lines_of_code,
                        f.tokens = row.tokens
                    RETURN count(f)
                    """,
                    {"rows": chunk},
                )
        except Exception:
            for row in payload:
                self.insert_file(
                    row["path"],
                    row["language"],
                    row["hash"],
                    row["lines_of_code"],
                    row["tokens"],
                )
        return stage_to_dest

    def bulk_insert_entities(self, rows: list[dict[str, Any]]) -> dict[int, int]:
        if not rows:
            return {}

        payload: list[dict[str, Any]] = []
        stage_to_dest: dict[int, int] = {}
        touched_file_ids: set[int] = set()
        for row in rows:
            stage_id = int(row["stage_id"])
            dest_id = self._alloc_id("Entity")
            stage_to_dest[stage_id] = dest_id
            file_id = int(row["file_id"])
            touched_file_ids.add(file_id)
            payload.append(
                {
                    "stage_id": stage_id,
                    "dest_id": dest_id,
                    "file_id": file_id,
                    "name": str(row["name"]),
                    "entity_type": str(row["entity_type"]),
                    "kind": str(row["kind"]),
                    "signature": str(row["signature"]),
                    "start_line": int(row["start_line"]),
                    "end_line": int(row["end_line"]),
                    "documentation": str(row["documentation"]),
                    "parent": str(row["parent"]),
                    "visibility": str(row["visibility"]),
                    "scope": str(row["scope"]),
                    "language": str(row["language"]),
                }
            )

        try:
            for chunk in self._chunked(payload):
                self._query(
                    """
                    UNWIND $rows AS row
                    MATCH (f:File {id: row.file_id})
                    CREATE (f)-[:CONTAINS]->(:Entity {
                      id: row.dest_id,
                      file_id: row.file_id,
                      name: row.name,
                      type: row.entity_type,
                      kind: row.kind,
                      signature: row.signature,
                      start_line: row.start_line,
                      end_line: row.end_line,
                      column_start: 0,
                      column_end: 0,
                      documentation: row.documentation,
                      parent: row.parent,
                      visibility: row.visibility,
                      scope: row.scope,
                      language: row.language,
                      attributes: ''
                    })
                    RETURN count(f)
                    """,
                    {"rows": chunk},
                )
        except Exception:
            stage_to_dest.clear()
            for row in payload:
                dest_id = self.insert_entity(
                    file_id=row["file_id"],
                    name=row["name"],
                    entity_type=row["entity_type"],
                    kind=row["kind"],
                    signature=row["signature"],
                    start_line=row["start_line"],
                    end_line=row["end_line"],
                    docs=row["documentation"],
                    parent=row["parent"],
                    visibility=row["visibility"],
                    scope=row["scope"],
                    language=row["language"],
                )
                stage_to_dest[int(row["stage_id"])] = dest_id
        for file_id in touched_file_ids:
            self._entity_key_cache.pop(file_id, None)
        return stage_to_dest

    def bulk_insert_dependencies(self, rows: list[dict[str, Any]]) -> int:
        if not rows:
            return 0

        payload: list[dict[str, Any]] = []
        touched_file_ids: set[int] = set()
        for row in rows:
            file_id = int(row["source_file_id"])
            touched_file_ids.add(file_id)
            payload.append(
                {
                    "dest_id": self._alloc_id("Dependency"),
                    "source_file_id": file_id,
                    "target_path": str(row["target_path"]),
                    "import_type": str(row["import_type"]),
                    "line_number": int(row["line_number"]),
                }
            )

        try:
            for chunk in self._chunked(payload):
                self._query(
                    """
                    UNWIND $rows AS row
                    MATCH (f:File {id: row.source_file_id})
                    CREATE (f)-[:HAS_DEP]->(:Dependency {
                      id: row.dest_id,
                      source_file_id: row.source_file_id,
                      target_path: row.target_path,
                      import_type: row.import_type,
                      line_number: row.line_number,
                      resolved: '',
                      is_local: 0
                    })
                    RETURN count(f)
                    """,
                    {"rows": chunk},
                )
            for file_id in touched_file_ids:
                self._dep_key_cache.pop(file_id, None)
            return len(payload)
        except Exception:
            for row in payload:
                self.insert_dependency(
                    row["source_file_id"],
                    row["target_path"],
                    row["import_type"],
                    row["line_number"],
                )
            return len(payload)

    def bulk_insert_relations(self, rows: list[dict[str, Any]]) -> int:
        if not rows:
            return 0

        payload = [
            {
                "dest_id": self._alloc_id("RELATES"),
                "source_id": int(row["source_entity_id"]),
                "target_id": int(row["target_entity_id"]),
                "relation_type": str(row["relation_type"]),
                "line_number": int(row["line_number"]),
                "context": str(row["context"]),
            }
            for row in rows
        ]

        try:
            for chunk in self._chunked(payload):
                self._query(
                    """
                    UNWIND $rows AS row
                    MATCH (s:Entity {id: row.source_id}), (t:Entity {id: row.target_id})
                    CREATE (s)-[:RELATES {
                      id: row.dest_id,
                      relation_type: row.relation_type,
                      line_number: row.line_number,
                      context: row.context
                    }]->(t)
                    RETURN count(s)
                    """,
                    {"rows": chunk},
                )
            return len(payload)
        except Exception:
            for row in payload:
                self.insert_entity_relation(
                    row["source_id"],
                    row["target_id"],
                    row["relation_type"],
                    row["line_number"],
                    row["context"],
                )
            return len(payload)

    def _ensure_entity_cache(self, file_id: int) -> dict[tuple[str, str, int], int]:
        cached = self._entity_key_cache.get(file_id)
        if cached is not None:
            return cached
        rows = self._query(
            """
            MATCH (:File {id: $file_id})-[:CONTAINS]->(e:Entity)
            RETURN e.name, e.type, e.start_line, e.id
            """,
            {"file_id": file_id},
        )
        mapped: dict[tuple[str, str, int], int] = {}
        for r in rows:
            mapped[(str(r[0]), str(r[1]), int(r[2]))] = int(r[3])
        self._entity_key_cache[file_id] = mapped
        return mapped

    def _ensure_dep_cache(self, file_id: int) -> dict[tuple[str, str, int], int]:
        cached = self._dep_key_cache.get(file_id)
        if cached is not None:
            return cached
        rows = self._query(
            """
            MATCH (:File {id: $file_id})-[:HAS_DEP]->(d:Dependency)
            RETURN d.target_path, d.import_type, d.line_number, d.id
            """,
            {"file_id": file_id},
        )
        mapped: dict[tuple[str, str, int], int] = {}
        for r in rows:
            mapped[(str(r[0]), str(r[1]), int(r[2]))] = int(r[3])
        self._dep_key_cache[file_id] = mapped
        return mapped

    def insert_file(self, path: str, language: str, file_hash: str, lines_of_code: int, tokens: int) -> int:
        rows = self._query("MATCH (f:File {path: $path}) RETURN f.id", {"path": path})
        if rows:
            file_id = int(rows[0][0])
            self._query(
                """
                MATCH (f:File {id: $id})
                SET f.language = $language,
                    f.hash = $hash,
                    f.lines_of_code = $loc,
                    f.tokens = $tokens
                RETURN f.id
                """,
                {
                    "id": file_id,
                    "language": language,
                    "hash": file_hash,
                    "loc": lines_of_code,
                    "tokens": tokens,
                },
            )
            return file_id

        file_id = self._alloc_id("File")
        self._query(
            """
            CREATE (f:File {
              id: $id,
              path: $path,
              language: $language,
              hash: $hash,
              lines_of_code: $loc,
              tokens: $tokens
            })
            RETURN f.id
            """,
            {
                "id": file_id,
                "path": path,
                "language": language,
                "hash": file_hash,
                "loc": lines_of_code,
                "tokens": tokens,
            },
        )
        return file_id

    def get_file_by_path(self, path: str) -> File | None:
        rows = self._query(
            """
            MATCH (f:File {path: $path})
            RETURN f.id, f.path, f.language, coalesce(f.hash, ''), coalesce(f.lines_of_code, 0), coalesce(f.tokens, 0)
            """,
            {"path": path},
        )
        if not rows:
            return None
        r = rows[0]
        return File(id=int(r[0]), path=str(r[1]), language=str(r[2]), hash=str(r[3]), lines_of_code=int(r[4]), tokens=int(r[5]))

    def get_file_by_id(self, file_id: int) -> File | None:
        rows = self._query(
            """
            MATCH (f:File {id: $id})
            RETURN f.id, f.path, f.language, coalesce(f.hash, ''), coalesce(f.lines_of_code, 0), coalesce(f.tokens, 0)
            """,
            {"id": file_id},
        )
        if not rows:
            return None
        r = rows[0]
        return File(id=int(r[0]), path=str(r[1]), language=str(r[2]), hash=str(r[3]), lines_of_code=int(r[4]), tokens=int(r[5]))

    def get_files(self) -> list[File]:
        rows = self._query(
            """
            MATCH (f:File)
            RETURN f.id, f.path, f.language, coalesce(f.hash, ''), coalesce(f.lines_of_code, 0), coalesce(f.tokens, 0)
            ORDER BY f.id ASC
            """
        )
        return [
            File(id=int(r[0]), path=str(r[1]), language=str(r[2]), hash=str(r[3]), lines_of_code=int(r[4]), tokens=int(r[5]))
            for r in rows
        ]

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
        cache = self._ensure_entity_cache(file_id)
        key = (name, entity_type, int(start_line))
        existing = cache.get(key)
        if existing is not None:
            return existing

        if self.get_file_by_id(file_id) is None:
            raise ValueError(f"file not found: {file_id}")

        entity_id = self._alloc_id("Entity")
        self._query(
            """
            MATCH (f:File {id: $file_id})
            CREATE (f)-[:CONTAINS]->(e:Entity {
              id: $id,
              file_id: $file_id,
              name: $name,
              type: $type,
              kind: $kind,
              signature: $signature,
              start_line: $start_line,
              end_line: $end_line,
              column_start: 0,
              column_end: 0,
              documentation: $documentation,
              parent: $parent,
              visibility: $visibility,
              scope: $scope,
              language: $language,
              attributes: ''
            })
            RETURN e.id
            """,
            {
                "id": entity_id,
                "file_id": file_id,
                "name": name,
                "type": entity_type,
                "kind": kind,
                "signature": signature,
                "start_line": start_line,
                "end_line": end_line,
                "documentation": docs,
                "parent": parent,
                "visibility": visibility,
                "scope": scope,
                "language": language,
            },
        )
        cache[key] = entity_id
        return entity_id

    def _entity_from_row(self, r: list[Any]) -> Entity:
        return Entity(
            id=int(r[0]),
            file_id=int(r[1]),
            name=str(r[2]),
            type=str(r[3]),
            kind=str(r[4]),
            signature=str(r[5]),
            start_line=int(r[6]),
            end_line=int(r[7]),
            column_start=int(r[8]),
            column_end=int(r[9]),
            documentation=str(r[10]),
            parent=str(r[11]),
            visibility=str(r[12]),
            scope=str(r[13]),
            language=str(r[14]),
            attributes=str(r[15]),
        )

    def get_entities_by_file(self, file_id: int) -> list[Entity]:
        rows = self._query(
            """
            MATCH (:File {id: $file_id})-[:CONTAINS]->(e:Entity)
            RETURN e.id, e.file_id, e.name, e.type, coalesce(e.kind, ''), coalesce(e.signature, ''),
                   coalesce(e.start_line, 0), coalesce(e.end_line, 0), coalesce(e.column_start, 0), coalesce(e.column_end, 0),
                   coalesce(e.documentation, ''), coalesce(e.parent, ''), coalesce(e.visibility, ''), coalesce(e.scope, ''),
                   coalesce(e.language, ''), coalesce(e.attributes, '')
            ORDER BY e.id ASC
            """,
            {"file_id": file_id},
        )
        return [self._entity_from_row(r) for r in rows]

    def get_entity_by_name(self, name: str) -> list[Entity]:
        rows = self._query(
            """
            MATCH (e:Entity {name: $name})
            RETURN e.id, e.file_id, e.name, e.type, coalesce(e.kind, ''), coalesce(e.signature, ''),
                   coalesce(e.start_line, 0), coalesce(e.end_line, 0), coalesce(e.column_start, 0), coalesce(e.column_end, 0),
                   coalesce(e.documentation, ''), coalesce(e.parent, ''), coalesce(e.visibility, ''), coalesce(e.scope, ''),
                   coalesce(e.language, ''), coalesce(e.attributes, '')
            ORDER BY e.id ASC
            """,
            {"name": name},
        )
        return [self._entity_from_row(r) for r in rows]

    def get_entity_by_id(self, entity_id: int) -> Entity | None:
        rows = self._query(
            """
            MATCH (e:Entity {id: $id})
            RETURN e.id, e.file_id, e.name, e.type, coalesce(e.kind, ''), coalesce(e.signature, ''),
                   coalesce(e.start_line, 0), coalesce(e.end_line, 0), coalesce(e.column_start, 0), coalesce(e.column_end, 0),
                   coalesce(e.documentation, ''), coalesce(e.parent, ''), coalesce(e.visibility, ''), coalesce(e.scope, ''),
                   coalesce(e.language, ''), coalesce(e.attributes, '')
            """,
            {"id": entity_id},
        )
        if not rows:
            return None
        return self._entity_from_row(rows[0])

    def get_all_entities(self) -> list[Entity]:
        rows = self._query(
            """
            MATCH (e:Entity)
            RETURN e.id, e.file_id, e.name, e.type, coalesce(e.kind, ''), coalesce(e.signature, ''),
                   coalesce(e.start_line, 0), coalesce(e.end_line, 0), coalesce(e.column_start, 0), coalesce(e.column_end, 0),
                   coalesce(e.documentation, ''), coalesce(e.parent, ''), coalesce(e.visibility, ''), coalesce(e.scope, ''),
                   coalesce(e.language, ''), coalesce(e.attributes, '')
            ORDER BY e.id ASC
            """
        )
        return [self._entity_from_row(r) for r in rows]

    def delete_entities_by_file(self, file_id: int) -> None:
        self._query("MATCH (:File {id: $file_id})-[:CONTAINS]->(e:Entity) DETACH DELETE e", {"file_id": file_id})
        self._entity_key_cache.pop(file_id, None)

    def delete_dependencies_by_file(self, file_id: int) -> None:
        self._query("MATCH (:File {id: $file_id})-[:HAS_DEP]->(d:Dependency) DETACH DELETE d", {"file_id": file_id})
        self._dep_key_cache.pop(file_id, None)

    def insert_dependency(self, source_file_id: int, target_path: str, dep_type: str, line_number: int) -> int:
        cache = self._ensure_dep_cache(source_file_id)
        key = (target_path, dep_type, int(line_number))
        existing = cache.get(key)
        if existing is not None:
            return existing

        dep_id = self._alloc_id("Dependency")
        self._query(
            """
            MATCH (f:File {id: $source_file_id})
            CREATE (f)-[:HAS_DEP]->(d:Dependency {
              id: $id,
              source_file_id: $source_file_id,
              target_path: $target_path,
              import_type: $import_type,
              line_number: $line_number,
              resolved: '',
              is_local: 0
            })
            RETURN d.id
            """,
            {
                "id": dep_id,
                "source_file_id": source_file_id,
                "target_path": target_path,
                "import_type": dep_type,
                "line_number": line_number,
            },
        )
        cache[key] = dep_id
        return dep_id

    def _dep_from_row(self, r: list[Any]) -> Dependency:
        return Dependency(
            id=int(r[0]),
            source_file_id=int(r[1]),
            target_path=str(r[2]),
            import_type=str(r[3]),
            line_number=int(r[4]),
            resolved=str(r[5]),
            is_local=bool(int(r[6])) if not isinstance(r[6], bool) else bool(r[6]),
        )

    def get_dependencies(self, file_id: int) -> list[Dependency]:
        rows = self._query(
            """
            MATCH (:File {id: $file_id})-[:HAS_DEP]->(d:Dependency)
            RETURN d.id, d.source_file_id, d.target_path, coalesce(d.import_type, ''), coalesce(d.line_number, 0),
                   coalesce(d.resolved, ''), coalesce(d.is_local, 0)
            ORDER BY d.id ASC
            """,
            {"file_id": file_id},
        )
        return [self._dep_from_row(r) for r in rows]

    def get_all_dependencies(self) -> list[Dependency]:
        rows = self._query(
            """
            MATCH (d:Dependency)
            RETURN d.id, d.source_file_id, d.target_path, coalesce(d.import_type, ''), coalesce(d.line_number, 0),
                   coalesce(d.resolved, ''), coalesce(d.is_local, 0)
            ORDER BY d.id ASC
            """
        )
        return [self._dep_from_row(r) for r in rows]

    def insert_entity_relation(
        self,
        source_entity_id: int,
        target_entity_id: int,
        relation_type: str,
        line_number: int,
        context: str,
    ) -> int:
        rel_id = self._alloc_id("RELATES")
        rows = self._query(
            """
            MATCH (s:Entity {id: $source_id}), (t:Entity {id: $target_id})
            MERGE (s)-[r:RELATES {relation_type: $relation_type}]->(t)
            ON CREATE SET
              r.id = $id,
              r.line_number = $line_number,
              r.context = $context
            RETURN r.id
            """,
            {
                "id": rel_id,
                "source_id": source_entity_id,
                "target_id": target_entity_id,
                "relation_type": relation_type,
                "line_number": line_number,
                "context": context,
            },
        )
        return int(rows[0][0]) if rows else rel_id

    def _rel_rows(self, query: str, params: dict[str, Any]) -> list[_RelRow]:
        rows = self._query(query, params)
        return [
            _RelRow(
                id=int(r[0]),
                source_id=int(r[1]),
                target_id=int(r[2]),
                relation_type=str(r[3]),
                line_number=int(r[4]),
                context=str(r[5]),
            )
            for r in rows
        ]

    def get_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        if relation_type:
            rows = self._rel_rows(
                """
                MATCH (s:Entity {id: $entity_id})-[r:RELATES]->(t:Entity)
                WHERE r.relation_type = $relation_type
                RETURN r.id, s.id, t.id, r.relation_type, coalesce(r.line_number, 0), coalesce(r.context, '')
                ORDER BY r.id ASC
                """,
                {"entity_id": entity_id, "relation_type": relation_type},
            )
        else:
            rows = self._rel_rows(
                """
                MATCH (s:Entity {id: $entity_id})-[r:RELATES]->(t:Entity)
                RETURN r.id, s.id, t.id, r.relation_type, coalesce(r.line_number, 0), coalesce(r.context, '')
                ORDER BY r.id ASC
                """,
                {"entity_id": entity_id},
            )
        return [
            EntityRelation(
                id=row.id,
                source_entity_id=row.source_id,
                target_entity_id=row.target_id,
                relation_type=row.relation_type,
                line_number=row.line_number,
                context=row.context,
            )
            for row in rows
        ]

    def get_inbound_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        if relation_type:
            rows = self._rel_rows(
                """
                MATCH (s:Entity)-[r:RELATES]->(t:Entity {id: $entity_id})
                WHERE r.relation_type = $relation_type
                RETURN r.id, s.id, t.id, r.relation_type, coalesce(r.line_number, 0), coalesce(r.context, '')
                ORDER BY r.id ASC
                """,
                {"entity_id": entity_id, "relation_type": relation_type},
            )
        else:
            rows = self._rel_rows(
                """
                MATCH (s:Entity)-[r:RELATES]->(t:Entity {id: $entity_id})
                RETURN r.id, s.id, t.id, r.relation_type, coalesce(r.line_number, 0), coalesce(r.context, '')
                ORDER BY r.id ASC
                """,
                {"entity_id": entity_id},
            )
        return [
            EntityRelation(
                id=row.id,
                source_entity_id=row.source_id,
                target_entity_id=row.target_id,
                relation_type=row.relation_type,
                line_number=row.line_number,
                context=row.context,
            )
            for row in rows
        ]

    def get_all_relations(self) -> list[EntityRelation]:
        rows = self._rel_rows(
            """
            MATCH (s:Entity)-[r:RELATES]->(t:Entity)
            RETURN r.id, s.id, t.id, r.relation_type, coalesce(r.line_number, 0), coalesce(r.context, '')
            ORDER BY r.id ASC
            """,
            {},
        )
        return [
            EntityRelation(
                id=row.id,
                source_entity_id=row.source_id,
                target_entity_id=row.target_id,
                relation_type=row.relation_type,
                line_number=row.line_number,
                context=row.context,
            )
            for row in rows
        ]

    def search_entities(self, query: str, limit: int = 20) -> list[Entity]:
        q = query.lower()
        rows = self._query(
            """
            MATCH (e:Entity)
            WHERE toLower(coalesce(e.name, '')) CONTAINS $q
               OR toLower(coalesce(e.type, '')) CONTAINS $q
               OR toLower(coalesce(e.parent, '')) CONTAINS $q
            RETURN e.id, e.file_id, e.name, e.type, coalesce(e.kind, ''), coalesce(e.signature, ''),
                   coalesce(e.start_line, 0), coalesce(e.end_line, 0), coalesce(e.column_start, 0), coalesce(e.column_end, 0),
                   coalesce(e.documentation, ''), coalesce(e.parent, ''), coalesce(e.visibility, ''), coalesce(e.scope, ''),
                   coalesce(e.language, ''), coalesce(e.attributes, '')
            ORDER BY e.name ASC, e.id ASC
            LIMIT $limit
            """,
            {"q": q, "limit": int(limit)},
        )
        return [self._entity_from_row(r) for r in rows]

    def get_file_imports(self, path: str) -> list[str]:
        file = self.get_file_by_path(path)
        if file is None:
            return []
        rows = self._query(
            """
            MATCH (:File {id: $file_id})-[:HAS_DEP]->(d:Dependency)
            RETURN d.target_path
            ORDER BY d.line_number ASC, d.id ASC
            """,
            {"file_id": file.id},
        )
        return [str(r[0]) for r in rows]

    def get_call_graph(self, entity_id: int, depth: int = 1) -> dict[str, Any]:
        _ = depth
        entity = self.get_entity_by_id(entity_id)
        if entity is None:
            raise ValueError("entity not found")

        rows = self._query(
            """
            MATCH (s:Entity {id: $entity_id})-[r:RELATES]->(t:Entity)
            WHERE r.relation_type = 'calls'
            RETURN t.id, t.name, t.type
            ORDER BY t.id ASC
            """,
            {"entity_id": entity_id},
        )
        calls = [{"id": int(r[0]), "name": str(r[1]), "type": str(r[2])} for r in rows]
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

    def get_file_count(self) -> int:
        rows = self._query("MATCH (f:File) RETURN count(f)")
        return int(rows[0][0]) if rows else 0

    def get_lines_of_code_count(self) -> int:
        rows = self._query("MATCH (f:File) RETURN coalesce(sum(f.lines_of_code), 0)")
        return int(rows[0][0]) if rows else 0

    def get_tokens_count(self) -> int:
        rows = self._query("MATCH (f:File) RETURN coalesce(sum(f.tokens), 0)")
        return int(rows[0][0]) if rows else 0

    def get_entity_count(self) -> int:
        rows = self._query("MATCH (e:Entity) RETURN count(e)")
        return int(rows[0][0]) if rows else 0

    def get_dependency_count(self) -> int:
        rows = self._query("MATCH (d:Dependency) RETURN count(d)")
        return int(rows[0][0]) if rows else 0

    def get_relation_count(self) -> int:
        rows = self._query("MATCH ()-[r:RELATES]->() RETURN count(r)")
        return int(rows[0][0]) if rows else 0
