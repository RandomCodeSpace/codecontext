from __future__ import annotations

import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass
class File:
    id: int
    path: str
    language: str
    hash: str
    lines_of_code: int
    tokens: int


@dataclass
class Entity:
    id: int
    file_id: int
    name: str
    type: str
    kind: str
    signature: str
    start_line: int
    end_line: int
    column_start: int
    column_end: int
    documentation: str
    parent: str
    visibility: str
    scope: str
    language: str
    attributes: str


@dataclass
class Dependency:
    id: int
    source_file_id: int
    target_path: str
    import_type: str
    line_number: int
    resolved: str
    is_local: bool


@dataclass
class EntityRelation:
    id: int
    source_entity_id: int
    target_entity_id: int
    relation_type: str
    line_number: int
    context: str


def _posix(path: str) -> str:
    """Normalize a file path to forward slashes for consistent storage/lookup."""
    return path.replace("\\", "/")


class Database:
    def __init__(self, conn: sqlite3.Connection):
        self.conn = conn
        self.conn.row_factory = sqlite3.Row

    @classmethod
    def open(cls, db_path: str, verbose: bool = False) -> "Database":
        _ = verbose
        path = Path(db_path)
        if path.parent and str(path.parent) not in ("", "."):
            path.parent.mkdir(parents=True, exist_ok=True)

        conn = sqlite3.connect(db_path, check_same_thread=False)
        conn.execute("PRAGMA foreign_keys = ON")
        conn.execute("PRAGMA journal_mode = WAL")
        conn.execute("PRAGMA busy_timeout = 5000")
        db = cls(conn)
        db.migrate()
        return db

    def close(self) -> None:
        self.conn.close()

    def migrate(self) -> None:
        self.conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS files (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                path TEXT NOT NULL UNIQUE,
                language TEXT NOT NULL,
                hash TEXT,
                lines_of_code INTEGER NOT NULL DEFAULT 0,
                tokens INTEGER NOT NULL DEFAULT 0
            );

            CREATE TABLE IF NOT EXISTS entities (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                file_id INTEGER NOT NULL,
                name TEXT NOT NULL,
                type TEXT NOT NULL,
                kind TEXT NOT NULL DEFAULT '',
                signature TEXT NOT NULL DEFAULT '',
                start_line INTEGER NOT NULL DEFAULT 0,
                end_line INTEGER NOT NULL DEFAULT 0,
                column_start INTEGER NOT NULL DEFAULT 0,
                column_end INTEGER NOT NULL DEFAULT 0,
                documentation TEXT NOT NULL DEFAULT '',
                parent TEXT NOT NULL DEFAULT '',
                visibility TEXT NOT NULL DEFAULT '',
                scope TEXT NOT NULL DEFAULT '',
                language TEXT NOT NULL DEFAULT '',
                attributes TEXT NOT NULL DEFAULT '',
                FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS dependencies (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                source_file_id INTEGER NOT NULL,
                target_path TEXT NOT NULL,
                import_type TEXT NOT NULL DEFAULT '',
                line_number INTEGER NOT NULL DEFAULT 0,
                resolved TEXT NOT NULL DEFAULT '',
                is_local INTEGER NOT NULL DEFAULT 0,
                FOREIGN KEY(source_file_id) REFERENCES files(id) ON DELETE CASCADE
            );

            CREATE TABLE IF NOT EXISTS entity_relations (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                source_entity_id INTEGER NOT NULL,
                target_entity_id INTEGER NOT NULL,
                relation_type TEXT NOT NULL,
                line_number INTEGER NOT NULL DEFAULT 0,
                context TEXT NOT NULL DEFAULT '',
                FOREIGN KEY(source_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
                FOREIGN KEY(target_entity_id) REFERENCES entities(id) ON DELETE CASCADE
            );

            CREATE INDEX IF NOT EXISTS idx_entities_file_id ON entities(file_id);
            CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
            CREATE INDEX IF NOT EXISTS idx_deps_source_file_id ON dependencies(source_file_id);
            CREATE INDEX IF NOT EXISTS idx_rel_source_id ON entity_relations(source_entity_id);
            CREATE INDEX IF NOT EXISTS idx_rel_target_id ON entity_relations(target_entity_id);
            """
        )
        self.conn.commit()

    def insert_file(self, path: str, language: str, file_hash: str, lines_of_code: int, tokens: int) -> int:
        normalized = _posix(path)
        row = self.conn.execute("SELECT id FROM files WHERE path = ?", (normalized,)).fetchone()
        if row is None:
            cur = self.conn.execute(
                "INSERT INTO files(path, language, hash, lines_of_code, tokens) VALUES (?, ?, ?, ?, ?)",
                (normalized, language, file_hash, lines_of_code, tokens),
            )
            return int(cur.lastrowid)

        file_id = int(row["id"])
        self.conn.execute(
            "UPDATE files SET language = ?, hash = ?, lines_of_code = ?, tokens = ? WHERE id = ?",
            (language, file_hash, lines_of_code, tokens, file_id),
        )
        return file_id

    def get_file_by_path(self, path: str) -> File | None:
        row = self.conn.execute(
            "SELECT id, path, language, hash, lines_of_code, tokens FROM files WHERE path = ?",
            (_posix(path),),
        ).fetchone()
        if row is None:
            return None
        return File(
            id=int(row["id"]),
            path=row["path"],
            language=row["language"],
            hash=row["hash"] or "",
            lines_of_code=int(row["lines_of_code"]),
            tokens=int(row["tokens"]),
        )

    def get_file_by_id(self, file_id: int) -> File | None:
        row = self.conn.execute(
            "SELECT id, path, language, hash, lines_of_code, tokens FROM files WHERE id = ?",
            (file_id,),
        ).fetchone()
        if row is None:
            return None
        return File(
            id=int(row["id"]),
            path=row["path"],
            language=row["language"],
            hash=row["hash"] or "",
            lines_of_code=int(row["lines_of_code"]),
            tokens=int(row["tokens"]),
        )

    def get_files(self) -> list[File]:
        rows = self.conn.execute("SELECT id, path, language, hash, lines_of_code, tokens FROM files").fetchall()
        return [
            File(
                id=int(row["id"]),
                path=row["path"],
                language=row["language"],
                hash=row["hash"] or "",
                lines_of_code=int(row["lines_of_code"]),
                tokens=int(row["tokens"]),
            )
            for row in rows
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
        row = self.conn.execute(
            """
            SELECT id FROM entities
            WHERE file_id = ? AND name = ? AND type = ? AND start_line = ?
            """,
            (file_id, name, entity_type, start_line),
        ).fetchone()
        if row is not None:
            return int(row["id"])

        cur = self.conn.execute(
            """
            INSERT INTO entities(file_id, name, type, kind, signature, start_line, end_line, documentation, parent, visibility, scope, language)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                file_id,
                name,
                entity_type,
                kind,
                signature,
                start_line,
                end_line,
                docs,
                parent,
                visibility,
                scope,
                language,
            ),
        )
        return int(cur.lastrowid)

    def get_entities_by_file(self, file_id: int) -> list[Entity]:
        rows = self.conn.execute(
            "SELECT * FROM entities WHERE file_id = ?",
            (file_id,),
        ).fetchall()
        return [self._row_to_entity(row) for row in rows]

    def get_entity_by_name(self, name: str) -> list[Entity]:
        rows = self.conn.execute("SELECT * FROM entities WHERE name = ?", (name,)).fetchall()
        return [self._row_to_entity(row) for row in rows]

    def get_entity_by_id(self, entity_id: int) -> Entity | None:
        row = self.conn.execute("SELECT * FROM entities WHERE id = ?", (entity_id,)).fetchone()
        if row is None:
            return None
        return self._row_to_entity(row)

    def get_all_entities(self) -> list[Entity]:
        rows = self.conn.execute("SELECT * FROM entities").fetchall()
        return [self._row_to_entity(row) for row in rows]

    def delete_entities_by_file(self, file_id: int) -> None:
        self.conn.execute("DELETE FROM entities WHERE file_id = ?", (file_id,))

    def delete_dependencies_by_file(self, file_id: int) -> None:
        self.conn.execute("DELETE FROM dependencies WHERE source_file_id = ?", (file_id,))

    def insert_dependency(self, source_file_id: int, target_path: str, dep_type: str, line_number: int) -> int:
        row = self.conn.execute(
            """
            SELECT id FROM dependencies
            WHERE source_file_id = ? AND target_path = ? AND import_type = ? AND line_number = ?
            """,
            (source_file_id, target_path, dep_type, line_number),
        ).fetchone()
        if row is not None:
            return int(row["id"])

        cur = self.conn.execute(
            "INSERT INTO dependencies(source_file_id, target_path, import_type, line_number) VALUES (?, ?, ?, ?)",
            (source_file_id, target_path, dep_type, line_number),
        )
        return int(cur.lastrowid)

    def get_dependencies(self, file_id: int) -> list[Dependency]:
        rows = self.conn.execute(
            "SELECT * FROM dependencies WHERE source_file_id = ?",
            (file_id,),
        ).fetchall()
        return [self._row_to_dependency(row) for row in rows]

    def get_all_dependencies(self) -> list[Dependency]:
        rows = self.conn.execute("SELECT * FROM dependencies").fetchall()
        return [self._row_to_dependency(row) for row in rows]

    def insert_entity_relation(
        self,
        source_entity_id: int,
        target_entity_id: int,
        relation_type: str,
        line_number: int,
        context: str,
    ) -> int:
        row = self.conn.execute(
            """
            SELECT id FROM entity_relations
            WHERE source_entity_id = ? AND target_entity_id = ? AND relation_type = ?
            """,
            (source_entity_id, target_entity_id, relation_type),
        ).fetchone()
        if row is not None:
            return int(row["id"])

        cur = self.conn.execute(
            """
            INSERT INTO entity_relations(source_entity_id, target_entity_id, relation_type, line_number, context)
            VALUES (?, ?, ?, ?, ?)
            """,
            (source_entity_id, target_entity_id, relation_type, line_number, context),
        )
        return int(cur.lastrowid)

    def get_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        if relation_type:
            rows = self.conn.execute(
                "SELECT * FROM entity_relations WHERE source_entity_id = ? AND relation_type = ?",
                (entity_id, relation_type),
            ).fetchall()
        else:
            rows = self.conn.execute(
                "SELECT * FROM entity_relations WHERE source_entity_id = ?",
                (entity_id,),
            ).fetchall()
        return [self._row_to_relation(row) for row in rows]

    def get_inbound_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        if relation_type:
            rows = self.conn.execute(
                "SELECT * FROM entity_relations WHERE target_entity_id = ? AND relation_type = ?",
                (entity_id, relation_type),
            ).fetchall()
        else:
            rows = self.conn.execute(
                "SELECT * FROM entity_relations WHERE target_entity_id = ?",
                (entity_id,),
            ).fetchall()
        return [self._row_to_relation(row) for row in rows]

    def get_all_relations(self) -> list[EntityRelation]:
        rows = self.conn.execute("SELECT * FROM entity_relations").fetchall()
        return [self._row_to_relation(row) for row in rows]

    def search_entities(self, query: str, limit: int = 20) -> list[Entity]:
        like = f"%{query}%"
        rows = self.conn.execute(
            """
            SELECT * FROM entities
            WHERE name LIKE ? OR type LIKE ? OR parent LIKE ?
            ORDER BY name ASC, id ASC
            LIMIT ?
            """,
            (like, like, like, limit),
        ).fetchall()
        return [self._row_to_entity(row) for row in rows]

    def get_file_imports(self, path: str) -> list[str]:
        file = self.get_file_by_path(_posix(path))
        if file is None:
            return []
        rows = self.conn.execute(
            "SELECT target_path FROM dependencies WHERE source_file_id = ? ORDER BY line_number ASC, id ASC",
            (file.id,),
        ).fetchall()
        return [str(row["target_path"]) for row in rows]

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
        normalized = _posix(file_path)
        file = self.get_file_by_path(normalized)
        if file is None:
            raise ValueError(f"file not found: {file_path}")
        deps = self.get_dependencies(file.id)
        return {"file": file.path, "dependencies": [dep.target_path for dep in deps]}

    def get_file_count(self) -> int:
        row = self.conn.execute("SELECT COUNT(*) AS c FROM files").fetchone()
        return int(row["c"])

    def get_lines_of_code_count(self) -> int:
        row = self.conn.execute("SELECT COALESCE(SUM(lines_of_code), 0) AS c FROM files").fetchone()
        return int(row["c"])

    def get_tokens_count(self) -> int:
        row = self.conn.execute("SELECT COALESCE(SUM(tokens), 0) AS c FROM files").fetchone()
        return int(row["c"])

    def get_entity_count(self) -> int:
        row = self.conn.execute("SELECT COUNT(*) AS c FROM entities").fetchone()
        return int(row["c"])

    def get_dependency_count(self) -> int:
        row = self.conn.execute("SELECT COUNT(*) AS c FROM dependencies").fetchone()
        return int(row["c"])

    def get_relation_count(self) -> int:
        row = self.conn.execute("SELECT COUNT(*) AS c FROM entity_relations").fetchone()
        return int(row["c"])

    def begin_bulk_operation(self) -> None:
        """Set SQLite PRAGMAs optimized for bulk writes (less durability, more speed)."""
        self.conn.execute("PRAGMA synchronous = OFF")
        self.conn.execute("PRAGMA cache_size = -64000")  # 64 MB
        self.conn.execute("PRAGMA temp_store = MEMORY")

    def end_bulk_operation(self) -> None:
        """Restore safe SQLite PRAGMAs after bulk writes."""
        self.conn.execute("PRAGMA synchronous = FULL")
        self.conn.execute("PRAGMA cache_size = -2000")  # default ~2 MB
        self.conn.execute("PRAGMA temp_store = DEFAULT")

    def bulk_sync_files(self, rows: list[dict[str, Any]]) -> dict[int, int]:
        """Bulk-insert files from staging. Returns {stage_id: dest_id} map."""
        id_map: dict[int, int] = {}
        for r in rows:
            stage_id = r["stage_id"]
            normalized = _posix(r["path"])
            existing = self.conn.execute(
                "SELECT id FROM files WHERE path = ?", (normalized,)
            ).fetchone()
            if existing is not None:
                file_id = int(existing["id"])
                self.conn.execute(
                    "DELETE FROM entities WHERE file_id = ?", (file_id,)
                )
                self.conn.execute(
                    "DELETE FROM dependencies WHERE source_file_id = ?", (file_id,)
                )
                self.conn.execute(
                    "UPDATE files SET language=?, hash=?, lines_of_code=?, tokens=? WHERE id=?",
                    (r["language"], r["hash"], r["lines_of_code"], r["tokens"], file_id),
                )
            else:
                cur = self.conn.execute(
                    "INSERT INTO files(path, language, hash, lines_of_code, tokens) VALUES (?,?,?,?,?)",
                    (normalized, r["language"], r["hash"], r["lines_of_code"], r["tokens"]),
                )
                file_id = int(cur.lastrowid)
            id_map[stage_id] = file_id
        return id_map

    def bulk_insert_entities(self, rows: list[dict[str, Any]]) -> dict[int, int]:
        """Bulk-insert entities from staging. Returns {stage_id: dest_id} map."""
        id_map: dict[int, int] = {}
        for r in rows:
            cur = self.conn.execute(
                """INSERT INTO entities(file_id, name, type, kind, signature,
                   start_line, end_line, documentation, parent, visibility,
                   scope, language)
                   VALUES (?,?,?,?,?,?,?,?,?,?,?,?)""",
                (
                    r["file_id"], r["name"], r["entity_type"], r.get("kind", ""),
                    r.get("signature", ""), r.get("start_line", 0),
                    r.get("end_line", 0), r.get("documentation", ""),
                    r.get("parent", ""), r.get("visibility", ""),
                    r.get("scope", ""), r.get("language", ""),
                ),
            )
            id_map[r["stage_id"]] = int(cur.lastrowid)
        return id_map

    def bulk_insert_dependencies(self, rows: list[dict[str, Any]]) -> int:
        """Bulk-insert dependencies from staging sync. Returns count."""
        if not rows:
            return 0
        params = [
            (r["source_file_id"], r["target_path"], r["import_type"], r.get("line_number", 0))
            for r in rows
        ]
        self.conn.executemany(
            "INSERT INTO dependencies(source_file_id, target_path, import_type, line_number) VALUES (?,?,?,?)",
            params,
        )
        return len(rows)

    def bulk_insert_relations(self, rows: list[dict[str, Any]]) -> int:
        """Bulk-insert relations from staging sync. Returns count."""
        if not rows:
            return 0
        params = [
            (r["source_entity_id"], r["target_entity_id"], r["relation_type"],
             r.get("line_number", 0), r.get("context", ""))
            for r in rows
        ]
        self.conn.executemany(
            """INSERT INTO entity_relations(source_entity_id, target_entity_id,
               relation_type, line_number, context) VALUES (?,?,?,?,?)""",
            params,
        )
        return len(rows)

    def batch_insert_entities(
        self,
        file_id: int,
        rows: list[dict[str, Any]],
    ) -> list[int]:
        """Insert multiple entities for a file using executemany. Skips
        duplicate checks — caller must ensure the file's entities have
        already been deleted.  Returns the list of assigned IDs."""
        if not rows:
            return []
        params = [
            (
                file_id,
                r["name"],
                r["entity_type"],
                r.get("kind", ""),
                r.get("signature", ""),
                r.get("start_line", 0),
                r.get("end_line", 0),
                r.get("docs", ""),
                r.get("parent", ""),
                r.get("visibility", ""),
                r.get("scope", ""),
                r.get("language", ""),
            )
            for r in rows
        ]
        self.conn.executemany(
            """INSERT INTO entities(file_id, name, type, kind, signature,
               start_line, end_line, documentation, parent, visibility,
               scope, language)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            params,
        )
        # Fetch back the IDs for the rows we just inserted (ordered by rowid).
        id_rows = self.conn.execute(
            "SELECT id FROM entities WHERE file_id = ? ORDER BY id",
            (file_id,),
        ).fetchall()
        return [int(r["id"]) for r in id_rows[-len(rows):]]

    def batch_insert_dependencies(
        self,
        file_id: int,
        rows: list[dict[str, Any]],
    ) -> int:
        """Insert multiple dependencies using executemany. Returns count."""
        if not rows:
            return 0
        params = [
            (file_id, r["path"], r["type"], r.get("line_number", 0))
            for r in rows
        ]
        self.conn.executemany(
            "INSERT INTO dependencies(source_file_id, target_path, import_type, line_number) VALUES (?, ?, ?, ?)",
            params,
        )
        return len(rows)

    def batch_insert_relations(
        self,
        rows: list[tuple[int, int, str, int, str]],
    ) -> int:
        """Insert multiple entity relations using executemany. Returns count."""
        if not rows:
            return 0
        self.conn.executemany(
            """INSERT INTO entity_relations(source_entity_id, target_entity_id,
               relation_type, line_number, context)
               VALUES (?, ?, ?, ?, ?)""",
            rows,
        )
        return len(rows)

    def commit(self) -> None:
        self.conn.commit()

    @staticmethod
    def _row_to_entity(row: sqlite3.Row) -> Entity:
        return Entity(
            id=int(row["id"]),
            file_id=int(row["file_id"]),
            name=row["name"],
            type=row["type"],
            kind=row["kind"],
            signature=row["signature"],
            start_line=int(row["start_line"]),
            end_line=int(row["end_line"]),
            column_start=int(row["column_start"]),
            column_end=int(row["column_end"]),
            documentation=row["documentation"],
            parent=row["parent"],
            visibility=row["visibility"],
            scope=row["scope"],
            language=row["language"],
            attributes=row["attributes"],
        )

    @staticmethod
    def _row_to_dependency(row: sqlite3.Row) -> Dependency:
        return Dependency(
            id=int(row["id"]),
            source_file_id=int(row["source_file_id"]),
            target_path=row["target_path"],
            import_type=row["import_type"],
            line_number=int(row["line_number"]),
            resolved=row["resolved"],
            is_local=bool(row["is_local"]),
        )

    @staticmethod
    def _row_to_relation(row: sqlite3.Row) -> EntityRelation:
        return EntityRelation(
            id=int(row["id"]),
            source_entity_id=int(row["source_entity_id"]),
            target_entity_id=int(row["target_entity_id"]),
            relation_type=row["relation_type"],
            line_number=int(row["line_number"]),
            context=row["context"],
        )
