from __future__ import annotations

from concurrent.futures import ProcessPoolExecutor, ThreadPoolExecutor, as_completed
import hashlib
import os
import signal
import sys
import time
from pathlib import Path
from typing import Any

from .db import Database, Entity
from .parser import ParseResult, parse
from .storage import StorageBackend


class Indexer:
    def __init__(self, database: StorageBackend, backend: str = "sqlite"):
        self.db = database
        self.backend = (backend or "sqlite").strip().lower()
        self.verbose = False
        self.parse_workers = _workers()
        self._stage_db: Database | None = None
        self._staging_enabled = False

    def set_verbose(self, verbose: bool) -> None:
        self.verbose = verbose

    def set_parse_workers(self, workers: int) -> None:
        self.parse_workers = max(1, int(workers))

    @staticmethod
    def _timestamp() -> str:
        return time.strftime("%Y-%m-%d %H:%M:%S")

    @staticmethod
    def _emoji_for(message: str) -> str:
        text = message.lower()
        if "failed" in text or "error" in text:
            return "❌"
        if "interrupt" in text or "canceled" in text:
            return "🛑"
        if "slow file" in text:
            return "🐢"
        if "scanning" in text:
            return "🧭"
        if "found" in text and "source files" in text:
            return "📁"
        if "precheck" in text:
            return "🧪"
        if "rate=" in text or "eta=" in text:
            return "📊"
        if "starting stage->destination sync" in text:
            return "🚚"
        if "phase complete" in text:
            return "✅"
        if text.startswith("[sync]"):
            return "🔄"
        if text.startswith("[index]"):
            return "🔍"
        if "skip" in text:
            return "⏭️"
        if "complete" in text:
            return "✅"
        return "ℹ️"

    def _format_log_line(self, message: str) -> str:
        return f"{self._timestamp()} {self._emoji_for(message)} {message}"

    def _log(self, message: str) -> None:
        if self.verbose:
            print(self._format_log_line(message), file=sys.stderr)

    def _progress(self, message: str) -> None:
        print(self._format_log_line(message), file=sys.stderr, flush=True)

    def index_file(self, file_path: str) -> None:
        normalized = Path(file_path).as_posix()
        probe = _probe_file_job(normalized)
        prepared = _prepare_file_job(
            file_path=normalized,
            content_hash=str(probe["hash"]),
            lines_of_code=int(probe["lines_of_code"]),
            tokens=int(probe["tokens"]),
        )
        self._apply_prepared(prepared)
        if self._use_staging():
            try:
                self._sync_staging_to_destination()
            finally:
                self._close_stage_db()

    def _use_staging(self) -> bool:
        return self._staging_enabled

    def _enable_staging(self, enabled: bool = True) -> None:
        self._staging_enabled = enabled

    def _get_stage_db(self) -> Database:
        if self._stage_db is None:
            self._stage_db = Database.open(":memory:", self.verbose)
        return self._stage_db

    def _close_stage_db(self) -> None:
        if self._stage_db is None:
            return
        self._stage_db.close()
        self._stage_db = None

    def _write_db(self) -> StorageBackend:
        if self._use_staging():
            return self._get_stage_db()
        return self.db

    def _apply_prepared(self, prepared: dict[str, object]) -> dict[str, int | bool]:
        path = Path(str(prepared["path"])).as_posix()
        content_hash = str(prepared["hash"])
        lines_of_code = int(prepared["lines_of_code"])
        tokens = int(prepared["tokens"])
        language = str(prepared["language"])
        entities = list(prepared["entities"])
        dependencies = list(prepared["dependencies"])

        existing = self.db.get_file_by_path(path)
        if existing is not None and existing.hash == content_hash:
            self._log(f"skip unchanged: {path}")
            return {
                "skipped": True,
                "skip_reason": "unchanged_hash",
                "path": path,
                "entities": 0,
                "dependencies": 0,
                "relations": 0,
            }

        write_db = self._write_db()
        existing_write = write_db.get_file_by_path(path)
        if existing_write is not None:
            write_db.delete_entities_by_file(existing_write.id)
            write_db.delete_dependencies_by_file(existing_write.id)

        file_id = write_db.insert_file(path, language, content_hash, lines_of_code, tokens)

        # -- Batch insert entities ----------------------------------------
        entity_rows = [
            {
                "name": str(e.get("name", "")),
                "entity_type": str(e.get("type", "")),
                "kind": str(e.get("kind", "")),
                "signature": str(e.get("signature", "")),
                "start_line": int(e.get("start_line", 0)),
                "end_line": int(e.get("end_line", 0)),
                "docs": str(e.get("docs", "")),
                "parent": str(e.get("parent", "")),
                "visibility": str(e.get("visibility", "")),
                "scope": str(e.get("scope", "")),
                "language": language,
            }
            for e in entities
        ]
        entity_ids = write_db.batch_insert_entities(file_id, entity_rows)

        # Build qualified-name -> id mapping for relations
        by_qualified_name: dict[str, int] = {}
        for entity_row, entity_id in zip(entity_rows, entity_ids):
            parent = entity_row["parent"]
            name = entity_row["name"]
            key = f"{parent}.{name}" if parent else name
            by_qualified_name.setdefault(key, entity_id)

        # -- Batch insert relations ----------------------------------------
        relation_rows: list[tuple[int, int, str, int, str]] = []
        for entity_row in entity_rows:
            parent = entity_row["parent"]
            if not parent:
                continue
            name = entity_row["name"]
            parent_id = by_qualified_name.get(parent)
            child_key = f"{parent}.{name}"
            child_id = by_qualified_name.get(child_key)
            if parent_id and child_id:
                relation_rows.append(
                    (parent_id, child_id, "defines", entity_row["start_line"], "")
                )

        write_db.batch_insert_relations(relation_rows)

        # -- Batch insert dependencies ------------------------------------
        dep_rows = [
            {
                "path": str(dep.get("path", "")),
                "type": str(dep.get("type", "")),
                "line_number": int(dep.get("line_number", 0)),
            }
            for dep in dependencies
        ]
        write_db.batch_insert_dependencies(file_id, dep_rows)

        write_db.commit()

        if self.verbose:
            self._log(
                f"indexed {path}: entities={len(entities)} deps={len(dependencies)} relations={len(relation_rows)}"
            )

        return {
            "skipped": False,
            "skip_reason": "",
            "path": path,
            "entities": len(entities),
            "dependencies": len(dependencies),
            "relations": len(relation_rows),
        }

    def _sync_staging_to_destination(self) -> None:
        if not self._use_staging() or self._stage_db is None:
            return

        stage = self._stage_db
        staged_files = stage.get_files()
        if not staged_files:
            return

        self._progress(f"[sync] starting stage->destination sync for {len(staged_files)} files")
        abort_requested = False
        prev_handler: object | None = None

        def _mark_abort(signum: int, frame: object) -> None:
            _ = signum, frame
            nonlocal abort_requested
            abort_requested = True
            self._progress("[sync] interrupt requested; finishing current phase before abort")

        try:
            prev_handler = signal.getsignal(signal.SIGINT)
            signal.signal(signal.SIGINT, _mark_abort)
        except Exception:
            prev_handler = None

        try:
            t0 = time.monotonic()
            bulk_sync_files = getattr(self.db, "bulk_sync_files", None)
            file_id_map: dict[int, int]
            if callable(bulk_sync_files):
                file_id_map = bulk_sync_files(
                    [
                        {
                            "stage_id": staged_file.id,
                            "path": staged_file.path,
                            "language": staged_file.language,
                            "hash": staged_file.hash,
                            "lines_of_code": staged_file.lines_of_code,
                            "tokens": staged_file.tokens,
                        }
                        for staged_file in staged_files
                    ]
                )
            else:
                file_id_map = {}
                for staged_file in staged_files:
                    existing = self.db.get_file_by_path(staged_file.path)
                    if existing is not None:
                        self.db.delete_entities_by_file(existing.id)
                        self.db.delete_dependencies_by_file(existing.id)
                    dest_file_id = self.db.insert_file(
                        staged_file.path,
                        staged_file.language,
                        staged_file.hash,
                        staged_file.lines_of_code,
                        staged_file.tokens,
                    )
                    file_id_map[staged_file.id] = dest_file_id
            self.db.commit()
            self._progress(f"[sync] files phase complete: {len(file_id_map)} in {time.monotonic() - t0:.1f}s")
            if abort_requested:
                raise KeyboardInterrupt()

            t1 = time.monotonic()
            stage_entities = stage.get_all_entities()
            bulk_insert_entities = getattr(self.db, "bulk_insert_entities", None)
            entity_id_map: dict[int, int]
            if callable(bulk_insert_entities):
                entity_rows: list[dict[str, Any]] = []
                for entity in stage_entities:
                    dest_file_id = file_id_map.get(entity.file_id)
                    if dest_file_id is None:
                        continue
                    entity_rows.append(
                        {
                            "stage_id": entity.id,
                            "file_id": dest_file_id,
                            "name": entity.name,
                            "entity_type": entity.type,
                            "kind": entity.kind,
                            "signature": entity.signature,
                            "start_line": entity.start_line,
                            "end_line": entity.end_line,
                            "documentation": entity.documentation,
                            "parent": entity.parent,
                            "visibility": entity.visibility,
                            "scope": entity.scope,
                            "language": entity.language,
                        }
                    )
                entity_id_map = bulk_insert_entities(entity_rows)
            else:
                entity_id_map = {}
                for entity in stage_entities:
                    dest_file_id = file_id_map.get(entity.file_id)
                    if dest_file_id is None:
                        continue
                    dest_entity_id = self.db.insert_entity(
                        file_id=dest_file_id,
                        name=entity.name,
                        entity_type=entity.type,
                        kind=entity.kind,
                        signature=entity.signature,
                        start_line=entity.start_line,
                        end_line=entity.end_line,
                        docs=entity.documentation,
                        parent=entity.parent,
                        visibility=entity.visibility,
                        scope=entity.scope,
                        language=entity.language,
                    )
                    entity_id_map[entity.id] = dest_entity_id
            self.db.commit()
            self._progress(
                f"[sync] entities phase complete: {len(entity_id_map)} in {time.monotonic() - t1:.1f}s"
            )
            if abort_requested:
                raise KeyboardInterrupt()

            t2 = time.monotonic()
            stage_deps = stage.get_all_dependencies()
            bulk_insert_dependencies = getattr(self.db, "bulk_insert_dependencies", None)
            if callable(bulk_insert_dependencies):
                dep_rows: list[dict[str, Any]] = []
                for dep in stage_deps:
                    dest_file_id = file_id_map.get(dep.source_file_id)
                    if dest_file_id is None:
                        continue
                    dep_rows.append(
                        {
                            "source_file_id": dest_file_id,
                            "target_path": dep.target_path,
                            "import_type": dep.import_type,
                            "line_number": dep.line_number,
                        }
                    )
                dep_count = int(bulk_insert_dependencies(dep_rows))
            else:
                dep_count = 0
                for dep in stage_deps:
                    dest_file_id = file_id_map.get(dep.source_file_id)
                    if dest_file_id is None:
                        continue
                    self.db.insert_dependency(dest_file_id, dep.target_path, dep.import_type, dep.line_number)
                    dep_count += 1
            self.db.commit()
            self._progress(f"[sync] dependencies phase complete: {dep_count} in {time.monotonic() - t2:.1f}s")
            if abort_requested:
                raise KeyboardInterrupt()

            t3 = time.monotonic()
            stage_rels = stage.get_all_relations()
            bulk_insert_relations = getattr(self.db, "bulk_insert_relations", None)
            if callable(bulk_insert_relations):
                rel_rows: list[dict[str, Any]] = []
                for rel in stage_rels:
                    source_id = entity_id_map.get(rel.source_entity_id)
                    target_id = entity_id_map.get(rel.target_entity_id)
                    if source_id is None or target_id is None:
                        continue
                    rel_rows.append(
                        {
                            "source_entity_id": source_id,
                            "target_entity_id": target_id,
                            "relation_type": rel.relation_type,
                            "line_number": rel.line_number,
                            "context": rel.context,
                        }
                    )
                rel_count = int(bulk_insert_relations(rel_rows))
            else:
                rel_count = 0
                for rel in stage_rels:
                    source_id = entity_id_map.get(rel.source_entity_id)
                    target_id = entity_id_map.get(rel.target_entity_id)
                    if source_id is None or target_id is None:
                        continue
                    self.db.insert_entity_relation(source_id, target_id, rel.relation_type, rel.line_number, rel.context)
                    rel_count += 1
            self.db.commit()
            self._progress(f"[sync] relations phase complete: {rel_count} in {time.monotonic() - t3:.1f}s")
        finally:
            if prev_handler is not None:
                try:
                    signal.signal(signal.SIGINT, prev_handler)
                except Exception:
                    pass

    def index_directory(self, dir_path: str) -> None:
        root = Path(dir_path)
        self._progress(f"[index] scanning source files under {root}")
        paths = [p for p in root.rglob("*") if p.is_file() and _is_source_file(p.suffix.lower()) and not _is_hidden(p)]
        total = len(paths)

        # Enable in-memory staging: all writes go to RAM, then bulk-sync to disk.
        use_staging = total > 10
        self._enable_staging(use_staging)
        staging_label = " staging=memory" if use_staging else ""

        mode = "parallel parse + serialized writes" if self.parse_workers > 1 else "sequential mode"
        self._progress(f"[index] found {total} source files; workers={self.parse_workers} ({mode}){staging_label}")

        if total == 0:
            return

        start = time.monotonic()
        last_report = start
        report_every = max(1, min(200, total // 20 or 1))
        skipped = 0
        skip_reasons: dict[str, int] = {}
        skip_examples: dict[str, list[str]] = {}
        changed_paths: list[Path] = []
        probes: dict[str, dict[str, Any]] = {}
        entities = 0
        dependencies = 0
        relations = 0

        def _record_skip(stats: dict[str, int | bool]) -> None:
            nonlocal skipped
            if not bool(stats.get("skipped", False)):
                return
            skipped += 1
            reason = str(stats.get("skip_reason", "unknown"))
            skip_reasons[reason] = skip_reasons.get(reason, 0) + 1
            ex = skip_examples.setdefault(reason, [])
            path = str(stats.get("path", ""))
            if path and len(ex) < 3:
                ex.append(path)

        def report_progress(done: int) -> None:
            nonlocal last_report
            now = time.monotonic()
            should_report = (
                done == 1
                or done == total
                or done % report_every == 0
                or (now - last_report) >= 2.0
            )
            if not should_report:
                return
            elapsed = now - start
            rate = done / elapsed if elapsed > 0 else 0.0
            remaining = max(total - done, 0)
            eta = (remaining / rate) if rate > 0 else 0.0
            pct = (done / total) * 100.0
            reason_summary = ""
            if skipped > 0:
                ordered = sorted(skip_reasons.items(), key=lambda item: item[1], reverse=True)
                top = ", ".join(f"{k}:{v}" for k, v in ordered[:3])
                reason_summary = f" skip_reasons={top}"
            self._progress(
                "[index] "
                f"{done}/{total} ({pct:.1f}%) "
                f"elapsed={elapsed:.1f}s eta={eta:.1f}s "
                f"rate={rate:.1f} files/s "
                f"skipped={skipped} entities={entities} deps={dependencies} relations={relations}"
                f"{reason_summary}"
            )
            last_report = now

        try:
            # Enable performance PRAGMAs for bulk operations on the destination DB.
            begin_bulk = getattr(self.db, "begin_bulk_operation", None)
            if callable(begin_bulk):
                begin_bulk()

            existing_hash_by_path = {f.path: f.hash for f in self.db.get_files()}
            precheck_start = time.monotonic()

            # Parallelize I/O-bound file reads + hashing with threads.
            probe_workers = max(1, min(self.parse_workers * 2, 16))
            posix_by_path = {id(p): p.as_posix() for p in paths}

            with ThreadPoolExecutor(max_workers=probe_workers) as tpool:
                future_to_path = {
                    tpool.submit(_probe_file_job, posix_by_path[id(p)]): p
                    for p in paths
                }
                for future in as_completed(future_to_path):
                    path = future_to_path[future]
                    posix_path = posix_by_path[id(path)]
                    try:
                        probe = future.result()
                    except Exception as err:  # noqa: BLE001
                        self._progress(f"[index] failed during precheck at {path}: {err}")
                        raise
                    probes[posix_path] = probe

                    existing_hash = existing_hash_by_path.get(posix_path, "")
                    if existing_hash and existing_hash == str(probe["hash"]):
                        _record_skip({"skipped": True, "skip_reason": "unchanged_hash", "path": posix_path})
                    else:
                        changed_paths.append(path)

            precheck_elapsed = time.monotonic() - precheck_start
            self._progress(
                "[index] precheck complete: "
                f"changed={len(changed_paths)} skipped={skipped} "
                f"elapsed={precheck_elapsed:.1f}s"
            )

            if not changed_paths:
                report_progress(total)
                for reason, count in sorted(skip_reasons.items(), key=lambda item: item[1], reverse=True):
                    examples = skip_examples.get(reason, [])
                    sample = f" examples={'; '.join(examples)}" if examples else ""
                    self._progress(f"[index] skipped reason {reason}: {count}{sample}")
                return

            if self.parse_workers <= 1:
                done = skipped
                for path in changed_paths:
                    try:
                        posix_path = path.as_posix()
                        t0 = time.monotonic()
                        prepared = _full_parse_job(posix_path)
                        stats = self._apply_prepared(prepared)
                        took = time.monotonic() - t0
                        if took >= 2.0:
                            self._progress(f"[index] slow file {path} took {took:.1f}s")
                    except Exception as err:  # noqa: BLE001
                        self._progress(f"[index] failed at {path}: {err}")
                        raise

                    _record_skip(stats)
                    entities += int(stats["entities"])
                    dependencies += int(stats["dependencies"])
                    relations += int(stats["relations"])
                    done += 1
                    report_progress(done)
            else:
                future_map: dict[object, Path] = {}
                done = skipped
                with ProcessPoolExecutor(max_workers=self.parse_workers) as pool:
                    for path in changed_paths:
                        posix_path = path.as_posix()
                        future = pool.submit(_full_parse_job, posix_path)
                        future_map[future] = path

                    for future in as_completed(future_map):
                        path = future_map[future]
                        try:
                            t0 = time.monotonic()
                            prepared = future.result()
                            stats = self._apply_prepared(prepared)
                            took = time.monotonic() - t0
                            if took >= 2.0:
                                self._progress(f"[index] slow file {path} took {took:.1f}s")
                        except Exception as err:  # noqa: BLE001
                            self._progress(f"[index] failed at {path}: {err}")
                            raise

                        done += 1
                        _record_skip(stats)
                        entities += int(stats["entities"])
                        dependencies += int(stats["dependencies"])
                        relations += int(stats["relations"])
                        report_progress(done)

            if self._use_staging():
                self._sync_staging_to_destination()

            if skipped > 0:
                for reason, count in sorted(skip_reasons.items(), key=lambda item: item[1], reverse=True):
                    examples = skip_examples.get(reason, [])
                    sample = f" examples={'; '.join(examples)}" if examples else ""
                    self._progress(f"[index] skipped reason {reason}: {count}{sample}")
        finally:
            self._close_stage_db()
            self._enable_staging(False)
            # Restore safe PRAGMAs.
            end_bulk = getattr(self.db, "end_bulk_operation", None)
            if callable(end_bulk):
                end_bulk()

    def query_entity(self, name: str) -> list[Entity]:
        return self.db.get_entity_by_name(name)

    def query_call_graph(self, entity_id: int) -> dict[str, object]:
        return self.db.get_call_graph(entity_id, 1)

    def query_dependency_graph(self, file_path: str) -> dict[str, object]:
        return self.db.get_dependency_graph(file_path)

    def get_stats(self) -> dict[str, int]:
        return {
            "files": self.db.get_file_count(),
            "entities": self.db.get_entity_count(),
            "dependencies": self.db.get_dependency_count(),
            "relations": self.db.get_relation_count(),
            "lines_of_code": self.db.get_lines_of_code_count(),
            "tokens": self.db.get_tokens_count(),
        }

    def get_all_files(self):
        return self.db.get_files()

    def get_all_entities(self):
        return self.db.get_all_entities()

    def get_entity_by_id(self, entity_id: int):
        return self.db.get_entity_by_id(entity_id)

    def get_file_by_id(self, file_id: int):
        return self.db.get_file_by_id(file_id)

    def get_all_relations(self):
        return self.db.get_all_relations()

    def get_all_dependencies(self):
        return self.db.get_all_dependencies()

    def get_entities_by_file(self, file_id: int):
        return self.db.get_entities_by_file(file_id)


def _is_hidden(path: Path) -> bool:
    return any(part.startswith(".") for part in path.parts)


def _probe_file_job(file_path: str) -> dict[str, object]:
    path = Path(file_path)
    content = path.read_text(encoding="utf-8", errors="replace")
    lines_of_code = content.count("\n") + (0 if content.endswith("\n") or not content else 1)
    tokens = len(content) // 4
    content_hash = hashlib.md5(content.encode("utf-8", errors="replace")).hexdigest()

    return {
        "path": path.as_posix(),
        "hash": content_hash,
        "lines_of_code": lines_of_code,
        "tokens": tokens,
    }


def _prepare_file_job(file_path: str, content_hash: str, lines_of_code: int, tokens: int) -> dict[str, object]:
    path = Path(file_path)
    content = path.read_text(encoding="utf-8", errors="replace")
    parsed: ParseResult = parse(path.as_posix(), content)

    entities = [
        {
            "name": e.name,
            "type": e.type,
            "kind": e.kind,
            "signature": e.signature,
            "start_line": e.start_line,
            "end_line": e.end_line,
            "docs": e.docs,
            "parent": e.parent,
            "visibility": e.visibility,
            "scope": e.scope,
        }
        for e in parsed.entities
    ]
    dependencies = [
        {
            "path": d.path,
            "type": d.type,
            "line_number": d.line_number,
        }
        for d in parsed.dependencies
    ]

    return {
        "path": path.as_posix(),
        "hash": content_hash,
        "lines_of_code": lines_of_code,
        "tokens": tokens,
        "language": parsed.language,
        "entities": entities,
        "dependencies": dependencies,
    }


def _full_parse_job(file_path: str) -> dict[str, object]:
    """Single-read job: hash + parse in one file read (eliminates double I/O)."""
    path = Path(file_path)
    content = path.read_text(encoding="utf-8", errors="replace")

    lines_of_code = content.count("\n") + (0 if content.endswith("\n") or not content else 1)
    tokens = len(content) // 4
    content_hash = hashlib.md5(content.encode("utf-8", errors="replace")).hexdigest()

    parsed: ParseResult = parse(path.as_posix(), content)

    entities = [
        {
            "name": e.name,
            "type": e.type,
            "kind": e.kind,
            "signature": e.signature,
            "start_line": e.start_line,
            "end_line": e.end_line,
            "docs": e.docs,
            "parent": e.parent,
            "visibility": e.visibility,
            "scope": e.scope,
        }
        for e in parsed.entities
    ]
    dependencies = [
        {
            "path": d.path,
            "type": d.type,
            "line_number": d.line_number,
        }
        for d in parsed.dependencies
    ]

    return {
        "path": path.as_posix(),
        "hash": content_hash,
        "lines_of_code": lines_of_code,
        "tokens": tokens,
        "language": parsed.language,
        "entities": entities,
        "dependencies": dependencies,
    }


def _is_source_file(ext: str) -> bool:
    return ext in {".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".java"}


def _workers() -> int:
    cpu = os.cpu_count() or 1
    return max(1, min(cpu, 8))